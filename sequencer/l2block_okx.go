package sequencer

import (
	"context"
	"fmt"
	"time"

	"github.com/0xPolygonHermez/zkevm-node/event"
	"github.com/0xPolygonHermez/zkevm-node/log"
	"github.com/0xPolygonHermez/zkevm-node/state"
)

// finalizeWIPL2Block closes the wip L2 block and opens a new one
func (f *finalizer) finalizeWIPL2Block_okx(ctx context.Context) {
	log.Debugf("finalizing WIP L2 block [%d]", f.wipL2Block.trackingNum)

	prevTimestamp := f.wipL2Block.timestamp
	prevL1InfoTreeIndex := f.wipL2Block.l1InfoTreeExitRoot.L1InfoTreeIndex

	f.closeWIPL2Block_okx(ctx)

	f.openNewWIPL2Block_okx(ctx, prevTimestamp, &prevL1InfoTreeIndex)
}

// closeWIPL2Block closes the wip L2 block
func (f *finalizer) closeWIPL2Block_okx(ctx context.Context) {
	log.Debugf("closing WIP L2 block [%d]", f.wipL2Block.trackingNum)

	f.wipBatch.countOfL2Blocks++

	err := f.processL2Block_okx(ctx, f.wipL2Block)
	if err != nil {
		// Dump L2Block info
		f.dumpL2Block(f.wipL2Block)
		f.Halt(ctx, fmt.Errorf("error processing L2 block [%d], error: %v", f.wipL2Block.trackingNum, err), false)
	}
	// We update imStateRoot (used in tx-by-tx execution) to the finalStateRoot that has been updated after process the WIP L2 Block
	f.wipBatch.imStateRoot = f.wipBatch.finalStateRoot

	f.wipL2Block = nil
}

// openNewWIPL2Block opens a new wip L2 block
func (f *finalizer) openNewWIPL2Block_okx(ctx context.Context, prevTimestamp uint64, prevL1InfoTreeIndex *uint32) {
	newL2Block := &L2Block{}

	// Tracking number
	f.l2BlockCounter++
	newL2Block.trackingNum = f.l2BlockCounter

	newL2Block.deltaTimestamp = uint32(uint64(now().Unix()) - prevTimestamp)
	newL2Block.timestamp = prevTimestamp + uint64(newL2Block.deltaTimestamp)

	newL2Block.transactions = []*TxTracker{}

	f.lastL1InfoTreeMux.Lock()
	newL2Block.l1InfoTreeExitRoot = f.lastL1InfoTree
	f.lastL1InfoTreeMux.Unlock()

	// Check if L1InfoTreeIndex has changed, in this case we need to use this index in the changeL2block instead of zero
	// If it's the first wip L2 block after starting sequencer (prevL1InfoTreeIndex == nil) then we retrieve the last GER and we check if it's
	// different from the GER of the current L1InfoTreeIndex (if the GER is different this means that the index also is different)
	if prevL1InfoTreeIndex == nil {
		lastGER, err := f.stateIntf.GetLatestBatchGlobalExitRoot(ctx, nil)
		if err == nil {
			newL2Block.l1InfoTreeExitRootChanged = (newL2Block.l1InfoTreeExitRoot.GlobalExitRoot.GlobalExitRoot != lastGER)
		} else {
			// If we got an error when getting the latest GER then we consider that the index has not changed and it will be updated the next time we have a new L1InfoTreeIndex
			log.Warnf("failed to get the latest CER when initializing the WIP L2 block, assuming L1InfoTreeIndex has not changed, error: %v", err)
		}
	} else {
		newL2Block.l1InfoTreeExitRootChanged = (newL2Block.l1InfoTreeExitRoot.L1InfoTreeIndex != *prevL1InfoTreeIndex)
	}

	f.wipL2Block = newL2Block

	log.Debugf("creating new WIP L2 block [%d], batch: %d, deltaTimestamp: %d, timestamp: %d, l1InfoTreeIndex: %d, l1InfoTreeIndexChanged: %v",
		f.wipL2Block.trackingNum, f.wipBatch.batchNumber, f.wipL2Block.deltaTimestamp, f.wipL2Block.timestamp, f.wipL2Block.l1InfoTreeExitRoot.L1InfoTreeIndex, f.wipL2Block.l1InfoTreeExitRootChanged)

	// Update imStateRoot
	oldIMStateRoot := f.wipBatch.imStateRoot
	f.wipBatch.imStateRoot = f.wipL2Block.imStateRoot
	f.wipL2Block.bytes = changeL2BlockSize

	log.Infof("created new WIP L2 block [%d], batch: %d, deltaTimestamp: %d, timestamp: %d, l1InfoTreeIndex: %d, l1InfoTreeIndexChanged: %v, oldStateRoot: %s, imStateRoot: %s, used counters: %s, reserved counters: %s",
		f.wipL2Block.trackingNum, f.wipBatch.batchNumber, f.wipL2Block.deltaTimestamp, f.wipL2Block.timestamp, f.wipL2Block.l1InfoTreeExitRoot.L1InfoTreeIndex,
		f.wipL2Block.l1InfoTreeExitRootChanged, oldIMStateRoot, f.wipL2Block.imStateRoot, f.logZKCounters(f.wipL2Block.usedZKCounters), f.logZKCounters(f.wipL2Block.reservedZKCounters))
}

// processL2Block process a L2 Block and adds it to the pendingL2BlocksToStore channel
func (f *finalizer) processL2Block_okx(ctx context.Context, l2Block *L2Block) error {
	startProcessing := time.Now()

	initialStateRoot := f.wipBatch.finalStateRoot

	log.Infof("processing L2 block [%d], batch: %d, deltaTimestamp: %d, timestamp: %d, l1InfoTreeIndex: %d, l1InfoTreeIndexChanged: %v, initialStateRoot: %s txs: %d",
		l2Block.trackingNum, f.wipBatch.batchNumber, l2Block.deltaTimestamp, l2Block.timestamp, l2Block.l1InfoTreeExitRoot.L1InfoTreeIndex,
		l2Block.l1InfoTreeExitRootChanged, initialStateRoot, len(l2Block.transactions))

	batchResponse, batchL2DataSize, err := f.executeL2Block(ctx, initialStateRoot, l2Block)

	if err != nil {
		return fmt.Errorf("failed to execute L2 block [%d], error: %v", l2Block.trackingNum, err)
	}

	if len(batchResponse.BlockResponses) != 1 {
		return fmt.Errorf("length of batchResponse.BlockRespones returned by the executor is %d and must be 1", len(batchResponse.BlockResponses))
	}

	blockResponse := batchResponse.BlockResponses[0]

	// Sanity check. Check blockResponse.TransactionsReponses match l2Block.Transactions length, order and tx hashes
	if len(blockResponse.TransactionResponses) != len(l2Block.transactions) {
		return fmt.Errorf("length of TransactionsResponses %d doesn't match length of l2Block.transactions %d", len(blockResponse.TransactionResponses), len(l2Block.transactions))
	}
	for i, txResponse := range blockResponse.TransactionResponses {
		if txResponse.TxHash != l2Block.transactions[i].Hash {
			return fmt.Errorf("blockResponse.TransactionsResponses[%d] hash %s doesn't match l2Block.transactions[%d] hash %s", i, txResponse.TxHash.String(), i, l2Block.transactions[i].Hash)
		}
		_, err = f.handleProcessTransactionResponse_okx(ctx, l2Block.transactions[i], txResponse, batchResponse.ReadWriteAddresses)
		if err != nil {
			log.Errorf("failed to process tx %s, error: %v", txResponse.TxHash, err)
		}
	}

	// Sanity check. Check blockResponse.timestamp matches l2block.timestamp
	if blockResponse.Timestamp != l2Block.timestamp {
		return fmt.Errorf("blockResponse.Timestamp %d doesn't match l2Block.timestamp %d", blockResponse.Timestamp, l2Block.timestamp)
	}

	l2Block.batchResponse = batchResponse

	// Update finalRemainingResources of the batch
	fits, overflowResource := f.wipBatch.finalRemainingResources.Fits(state.BatchResources{ZKCounters: batchResponse.ReservedZkCounters, Bytes: batchL2DataSize})
	if fits {
		subOverflow, overflowResource := f.wipBatch.finalRemainingResources.Sub(state.BatchResources{ZKCounters: batchResponse.UsedZkCounters, Bytes: batchL2DataSize})
		if subOverflow { // Sanity check, this cannot happen as reservedZKCounters should be >= that usedZKCounters
			return fmt.Errorf("error subtracting L2 block %d [%d] used resources from the batch %d, overflow resource: %s, batch counters: %s, L2 block used counters: %s, batch bytes: %d, L2 block bytes: %d",
				blockResponse.BlockNumber, l2Block.trackingNum, f.wipBatch.batchNumber, overflowResource, f.logZKCounters(f.wipBatch.finalRemainingResources.ZKCounters), f.logZKCounters(batchResponse.UsedZkCounters), f.wipBatch.finalRemainingResources.Bytes, batchL2DataSize)
		}
	} else {
		overflowLog := fmt.Sprintf("L2 block %d [%d] reserved resources exceeds the remaining batch %d resources, overflow resource: %s, batch counters: %s, L2 block reserved counters: %s, batch bytes: %d, L2 block bytes: %d",
			blockResponse.BlockNumber, l2Block.trackingNum, f.wipBatch.batchNumber, overflowResource, f.logZKCounters(f.wipBatch.finalRemainingResources.ZKCounters), f.logZKCounters(batchResponse.ReservedZkCounters), f.wipBatch.finalRemainingResources.Bytes, batchL2DataSize)

		log.Warnf(overflowLog)

		f.LogEvent(ctx, event.Level_Warning, event.EventID_ReservedZKCountersOverflow, overflowLog, nil)
	}

	// Update finalStateRoot of the batch to the newStateRoot for the L2 block
	f.wipBatch.finalStateRoot = l2Block.batchResponse.NewStateRoot

	f.updateFlushIDs(batchResponse.FlushID, batchResponse.StoredFlushID)

	f.addPendingL2BlockToStore(ctx, l2Block)

	endProcessing := time.Now()

	log.Infof("processed L2 block %d [%d], batch: %d, deltaTimestamp: %d, timestamp: %d, l1InfoTreeIndex: %d, l1InfoTreeIndexChanged: %v, initialStateRoot: %s, newStateRoot: %s, txs: %d/%d, blockHash: %s, infoRoot: %s, time: %v, used counters: %s, reserved counters: %s",
		blockResponse.BlockNumber, l2Block.trackingNum, f.wipBatch.batchNumber, l2Block.deltaTimestamp, l2Block.timestamp, l2Block.l1InfoTreeExitRoot.L1InfoTreeIndex, l2Block.l1InfoTreeExitRootChanged, initialStateRoot, l2Block.batchResponse.NewStateRoot,
		len(l2Block.transactions), len(blockResponse.TransactionResponses), blockResponse.BlockHash, blockResponse.BlockInfoRoot, endProcessing.Sub(startProcessing), f.logZKCounters(batchResponse.UsedZkCounters), f.logZKCounters(batchResponse.ReservedZkCounters))

	return nil
}
