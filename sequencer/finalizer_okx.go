package sequencer

import (
	"context"
	"fmt"
	"github.com/0xPolygonHermez/zkevm-node/pool"
	"github.com/0xPolygonHermez/zkevm-node/state/runtime/executor"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"sync"
	"time"

	"github.com/0xPolygonHermez/zkevm-node/log"
	"github.com/0xPolygonHermez/zkevm-node/sequencer/metrics"
	"github.com/0xPolygonHermez/zkevm-node/state"
)

// finalizeBatches runs the endless loop for processing transactions finalizing batches.
func (f *finalizer) finalizeBatches_okx(ctx context.Context) {
	log.Debug("finalizer init loop")
	for {
		start := now()
		// We have reached the L2 block time, we need to close the current L2 block and open a new one
		if f.wipL2Block.timestamp+uint64(f.cfg.L2BlockMaxDeltaTimestamp.Seconds()) <= uint64(time.Now().Unix()) {
			f.finalizeWIPL2Block_okx(ctx)
		}

		t1 := now()
		txs, err := f.workerIntf.GetBestFittingTxs(f.wipBatch.imRemainingResources, 50)
		metrics.GetTxFromPoolTime(time.Since(t1))

		// If we have txs pending to process but none of them fits into the wip batch, we close the wip batch and open a new one
		if err == ErrNoFittingTransaction {
			f.finalizeWIPBatch(ctx, state.NoTxFitsClosingReason)
			continue
		}

		metrics.ProcessingTime(time.Since(start))
		metrics.WorkerProcessingTime(time.Since(start))
		if len(txs) > 0 {
			t1 := time.Now()
			for _, tx := range txs {
				if err := f.prepareTransaction(tx, true); err == nil {
					f.wipL2Block.addTx(tx)
					f.wipBatch.countOfTxs++
				} else {
					log.Errorf("failed to prepare tx %s, error: %v", err)
				}
			}

			f.finalizeWIPL2Block_okx(ctx)

			metrics.ProcessingTime(time.Since(t1))
		} else {
			if f.cfg.NewTxsWaitInterval.Duration > 0 {
				time.Sleep(f.cfg.NewTxsWaitInterval.Duration)
			}
		}

		if f.haltFinalizer.Load() {
			// There is a fatal error and we need to halt the finalizer and stop processing new txs
			for {
				time.Sleep(5 * time.Second) //nolint:gomnd
			}
		}

		t2 := time.Now()
		// Check if we must finalize the batch due to a closing reason (resources exhausted, max txs, timestamp resolution, forced batches deadline)
		if finalize, closeReason := f.checkIfFinalizeBatch(); finalize {
			f.finalizeWIPBatch(ctx, closeReason)
		}
		metrics.ProcessingTime(time.Since(t2))

		if err := ctx.Err(); err != nil {
			log.Errorf("stopping finalizer because of context, error: %v", err)
			return
		}
	}
}

func (f *finalizer) prepareTransaction(tx *TxTracker, firstTxProcess bool) error {
	txGasPrice := tx.GasPrice

	// If it is the first time we process this tx then we calculate the EffectiveGasPrice
	if firstTxProcess {
		// Get L1 gas price and store in txTracker to make it consistent during the lifespan of the transaction
		tx.L1GasPrice, tx.L2GasPrice = f.poolIntf.GetL1AndL2GasPrice()
		// Get the tx and l2 gas price we will use in the egp calculation. If egp is disabled we will use a "simulated" tx gas price
		txGasPrice, txL2GasPrice := f.effectiveGasPrice.GetTxAndL2GasPrice(tx.GasPrice, tx.L1GasPrice, tx.L2GasPrice)

		// Save values for later logging
		tx.EGPLog.L1GasPrice = tx.L1GasPrice
		tx.EGPLog.L2GasPrice = txL2GasPrice
		tx.EGPLog.GasUsedFirst = tx.UsedZKCounters.GasUsed
		tx.EGPLog.GasPrice.Set(txGasPrice)

		// Calculate EffectiveGasPrice
		egp, err := f.effectiveGasPrice.CalculateEffectiveGasPrice(tx.RawTx, txGasPrice, tx.UsedZKCounters.GasUsed, tx.L1GasPrice, txL2GasPrice)
		if err != nil {
			if f.effectiveGasPrice.IsEnabled() {
				return err
			} else {
				log.Warnf("effectiveGasPrice is disabled, but failed to calculate effectiveGasPrice for tx %s, error: %v", tx.HashStr, err)
				tx.EGPLog.Error = fmt.Sprintf("CalculateEffectiveGasPrice#1: %s", err)
			}
		} else {
			tx.EffectiveGasPrice.Set(egp)

			// Save first EffectiveGasPrice for later logging
			tx.EGPLog.ValueFirst.Set(tx.EffectiveGasPrice)

			// If EffectiveGasPrice >= txGasPrice, we process the tx with tx.GasPrice
			if tx.EffectiveGasPrice.Cmp(txGasPrice) >= 0 {
				loss := new(big.Int).Sub(tx.EffectiveGasPrice, txGasPrice)
				// If loss > 0 the warning message indicating we loss fee for thix tx
				if loss.Cmp(new(big.Int).SetUint64(0)) == 1 {
					log.Warnf("egp-loss: gasPrice: %d, effectiveGasPrice1: %d, loss: %d, tx: %s", txGasPrice, tx.EffectiveGasPrice, loss, tx.HashStr)
				}

				tx.EffectiveGasPrice.Set(txGasPrice)
				tx.IsLastExecution = true
			}
		}
	}

	egpPercentage, err := f.effectiveGasPrice.CalculateEffectiveGasPricePercentage(txGasPrice, tx.EffectiveGasPrice)
	if err != nil {
		if f.effectiveGasPrice.IsEnabled() {
			return err
		} else {
			log.Warnf("effectiveGasPrice is disabled, but failed to to calculate efftive gas price percentage (#1), error: %v", err)
			tx.EGPLog.Error = fmt.Sprintf("%s; CalculateEffectiveGasPricePercentage#1: %s", tx.EGPLog.Error, err)
		}
	} else {
		// Save percentage for later logging
		tx.EGPLog.Percentage = egpPercentage
	}

	// If EGP is disabled we use tx GasPrice (MaxEffectivePercentage=255)
	if !f.effectiveGasPrice.IsEnabled() {
		egpPercentage = state.MaxEffectivePercentage
	}

	// Assign applied EGP percentage to tx (TxTracker)
	tx.EGPPercentage = egpPercentage

	return nil
}

// handleProcessTransactionResponse handles the response of transaction processing.
func (f *finalizer) handleProcessTransactionResponse_okx(ctx context.Context, tx *TxTracker, result *state.ProcessTransactionResponse,
	touchedAddresses map[common.Address]*state.InfoReadWrite) (errWg *sync.WaitGroup, err error) {
	// Handle Transaction Error
	errorCode := executor.RomErrorCode(result.RomError)
	if !state.IsStateRootChanged(errorCode) {
		// If intrinsic error or OOC error, we skip adding the transaction to the batch
		errWg = f.handleProcessTransactionError_okx(ctx, result, tx, touchedAddresses)
		return errWg, result.RomError
	}

	egpEnabled := f.effectiveGasPrice.IsEnabled()

	if !tx.IsLastExecution {
		tx.IsLastExecution = true

		// Get the tx gas price we will use in the egp calculation. If egp is disabled we will use a "simulated" tx gas price
		txGasPrice, txL2GasPrice := f.effectiveGasPrice.GetTxAndL2GasPrice(tx.GasPrice, tx.L1GasPrice, tx.L2GasPrice)

		newEffectiveGasPrice, err := f.effectiveGasPrice.CalculateEffectiveGasPrice(tx.RawTx, txGasPrice, result.GasUsed, tx.L1GasPrice, txL2GasPrice)
		if err != nil {
			if egpEnabled {
				log.Errorf("failed to calculate effective gas price with new gasUsed for tx %s, error: %v", tx.HashStr, err.Error())
				return nil, err
			} else {
				log.Warnf("effectiveGasPrice is disabled, but failed to calculate effective gas price with new gasUsed for tx %s, error: %v", tx.HashStr, err.Error())
				tx.EGPLog.Error = fmt.Sprintf("%s; CalculateEffectiveGasPrice#2: %s", tx.EGPLog.Error, err)
			}
		} else {
			// Save new (second) gas used and second effective gas price calculation for later logging
			tx.EGPLog.ValueSecond.Set(newEffectiveGasPrice)
			tx.EGPLog.GasUsedSecond = result.GasUsed

			errCompare := f.compareTxEffectiveGasPrice(ctx, tx, newEffectiveGasPrice, result.HasGaspriceOpcode, result.HasBalanceOpcode)

			// If EffectiveGasPrice is disabled we will calculate the percentage and save it for later logging
			if !egpEnabled {
				effectivePercentage, err := f.effectiveGasPrice.CalculateEffectiveGasPricePercentage(txGasPrice, tx.EffectiveGasPrice)
				if err != nil {
					log.Warnf("effectiveGasPrice is disabled, but failed to calculate effective gas price percentage (#2), error: %v", err)
					tx.EGPLog.Error = fmt.Sprintf("%s, CalculateEffectiveGasPricePercentage#2: %s", tx.EGPLog.Error, err)
				} else {
					// Save percentage for later logging
					tx.EGPLog.Percentage = effectivePercentage
				}
			}

			if errCompare != nil && egpEnabled {
				return nil, errCompare
			}
		}
	}

	// Save Enabled, GasPriceOC, BalanceOC and final effective gas price for later logging
	tx.EGPLog.Enabled = egpEnabled
	tx.EGPLog.GasPriceOC = result.HasGaspriceOpcode
	tx.EGPLog.BalanceOC = result.HasBalanceOpcode
	tx.EGPLog.ValueFinal.Set(tx.EffectiveGasPrice)

	// Log here the results of EGP calculation
	log.Infof("egp-log: final: %d, first: %d, second: %d, percentage: %d, deviation: %d, maxDeviation: %d, gasUsed1: %d, gasUsed2: %d, gasPrice: %d, l1GasPrice: %d, l2GasPrice: %d, reprocess: %t, gasPriceOC: %t, balanceOC: %t, enabled: %t, txSize: %d, tx: %s, error: %s",
		tx.EGPLog.ValueFinal, tx.EGPLog.ValueFirst, tx.EGPLog.ValueSecond, tx.EGPLog.Percentage, tx.EGPLog.FinalDeviation, tx.EGPLog.MaxDeviation, tx.EGPLog.GasUsedFirst, tx.EGPLog.GasUsedSecond,
		tx.EGPLog.GasPrice, tx.EGPLog.L1GasPrice, tx.EGPLog.L2GasPrice, tx.EGPLog.Reprocess, tx.EGPLog.GasPriceOC, tx.EGPLog.BalanceOC, egpEnabled, len(tx.RawTx), tx.HashStr, tx.EGPLog.Error)

	f.updateWorkerAfterSuccessfulProcessing_okx(ctx, tx.Hash, tx.From, false, touchedAddresses)

	return nil, nil
}

// handleProcessTransactionError handles the error of a transaction
func (f *finalizer) handleProcessTransactionError_okx(ctx context.Context, result *state.ProcessTransactionResponse, tx *TxTracker,
	touchedAddresses map[common.Address]*state.InfoReadWrite) *sync.WaitGroup {
	errorCode := executor.RomErrorCode(result.RomError)
	addressInfo := touchedAddresses[tx.From]
	log.Infof("rom error in tx %s, errorCode: %d", tx.HashStr, errorCode)
	wg := new(sync.WaitGroup)
	failedReason := executor.RomErr(errorCode).Error()
	if executor.IsROMOutOfCountersError(errorCode) {
		log.Errorf("ROM out of counters error, marking tx %s as invalid, errorCode: %d", tx.HashStr, errorCode)
		start := time.Now()
		f.workerIntf.DeleteTx(tx.Hash, tx.From)
		metrics.WorkerProcessingTime(time.Since(start))

		wg.Add(1)
		go func() {
			defer wg.Done()
			err := f.poolIntf.UpdateTxStatus(ctx, tx.Hash, pool.TxStatusInvalid, false, &failedReason)
			if err != nil {
				log.Errorf("failed to update status to invalid in the pool for tx %s, error: %v", tx.HashStr, err)
			} else {
				metrics.TxProcessed(metrics.TxProcessedLabelInvalid, 1)
			}
		}()
	} else if executor.IsInvalidNonceError(errorCode) || executor.IsInvalidBalanceError(errorCode) {
		var (
			nonce   *uint64
			balance *big.Int
		)
		if addressInfo != nil {
			nonce = addressInfo.Nonce
			balance = addressInfo.Balance
		}
		start := time.Now()
		log.Errorf("intrinsic error, moving tx %s to not ready: nonce: %d, balance: %d. gasPrice: %d, error: %v", tx.Hash, nonce, balance, tx.GasPrice, result.RomError)
		txsToDelete := f.workerIntf.MoveTxToNotReady(tx.Hash, tx.From, nonce, balance)
		for _, txToDelete := range txsToDelete {
			wg.Add(1)
			txToDelete := txToDelete
			go func() {
				defer wg.Done()
				err := f.poolIntf.UpdateTxStatus(ctx, txToDelete.Hash, pool.TxStatusFailed, false, &failedReason)
				metrics.TxProcessed(metrics.TxProcessedLabelFailed, 1)
				if err != nil {
					log.Errorf("failed to update status to failed in the pool for tx %s, error: %v", txToDelete.Hash.String(), err)
				}
			}()
		}
		metrics.WorkerProcessingTime(time.Since(start))
	} else {
		// Delete the transaction from the txSorted list
		f.workerIntf.DeleteTx(tx.Hash, tx.From)
		log.Debugf("tx %s deleted from txSorted list", tx.HashStr)

		wg.Add(1)
		go func() {
			defer wg.Done()
			// Update the status of the transaction to failed
			err := f.poolIntf.UpdateTxStatus(ctx, tx.Hash, pool.TxStatusFailed, false, &failedReason)
			if err != nil {
				log.Errorf("failed to update status to failed in the pool for tx %s, error: %v", tx.Hash.String(), err)
			} else {
				metrics.TxProcessed(metrics.TxProcessedLabelFailed, 1)
			}
		}()
	}

	return wg
}

func (f *finalizer) updateWorkerAfterSuccessfulProcessing_okx(ctx context.Context, txHash common.Hash, txFrom common.Address,
	isForced bool, touchedAddresses map[common.Address]*state.InfoReadWrite) {
	// Delete the transaction from the worker
	if isForced {
		f.workerIntf.DeleteForcedTx(txHash, txFrom)
		log.Debugf("forced tx %s deleted from address %s", txHash.String(), txFrom.Hex())
		return
	} else {
		f.workerIntf.DeleteTx(txHash, txFrom)
		log.Debugf("tx %s deleted from address %s", txHash.String(), txFrom.Hex())
	}

	start := time.Now()
	txsToDelete := f.workerIntf.UpdateAfterSingleSuccessfulTxExecution(txFrom, touchedAddresses)
	for _, txToDelete := range txsToDelete {
		err := f.poolIntf.UpdateTxStatus(ctx, txToDelete.Hash, pool.TxStatusFailed, false, txToDelete.FailedReason)
		if err != nil {
			log.Errorf("failed to update status to failed in the pool for tx %s, error: %v", txToDelete.Hash.String(), err)
			continue
		}
		metrics.TxProcessed(metrics.TxProcessedLabelFailed, 1)
	}
	metrics.WorkerProcessingTime(time.Since(start))
}
