package sequencer

import (
	"github.com/0xPolygonHermez/zkevm-node/log"
	"github.com/0xPolygonHermez/zkevm-node/state"
)

// GetBestFittingTx gets the most efficient tx that fits in the available batch resources
func (w *Worker) GetBestFittingTxs(resources state.BatchResources, number int) ([]*TxTracker, error) {
	w.workerMutex.Lock()
	defer w.workerMutex.Unlock()

	if w.txSortedList.len() == 0 {
		return nil, ErrTransactionsListEmpty
	}

	txs := make([]*TxTracker, 0, number)
	targetLen := min(number*2, w.txSortedList.len())
	// we do not want to go through all list tx, and we need try our best to get the txCandidate
	for i := 0; i < targetLen; i++ {
		if len(txs) >= number {
			break
		}

		txCandidate := w.txSortedList.getByIndex(i)
		overflow, _ := resources.Sub(state.BatchResources{ZKCounters: txCandidate.ReservedZKCounters, Bytes: txCandidate.Bytes})
		if overflow {
			// We don't add this Tx
			continue
		}
		txs = append(txs, txCandidate)
	}

	if len(txs) > 0 {
		log.Debugf("get best fitting txs: %d", len(txs))
		return txs, nil
	} else {
		return nil, ErrNoFittingTransaction
	}
}
