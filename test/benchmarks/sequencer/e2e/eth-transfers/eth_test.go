package eth_transfers

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygonHermez/zkevm-node/pool"

	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/metrics"
	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/params"
	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/setup"
	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/transactions"
	"github.com/stretchr/testify/require"
)

const (
	profilingEnabled = false
)

func BenchmarkSequencerEthTransfersPoolProcess(b *testing.B) {
	var (
		elapsed time.Duration
	)

	start := time.Now()
	//defer func() { require.NoError(b, operations.Teardown()) }()
	opsman, client, pl, auth := setup.Environment(params.Ctx, b)
	setup.BootstrapSequencer(b, opsman)

	initialCount, err := pl.CountTransactionsByStatus(params.Ctx, pool.TxStatusSelected)
	require.NoError(b, err)

	authList := loadSenderAddr(client, "./addr_200")
	cnt := initSender(client, auth, authList, big.NewInt(1000*100000*1000000000), pl.GetTxsByStatus)
	err = transactions.WaitStatusSelected(pl.CountTransactionsByStatus, initialCount, uint64(cnt))
	require.NoError(b, err)
	elapsed = time.Since(start)
	fmt.Printf("Total elapsed time: %s\n", elapsed)

	initialCount, err = pl.CountTransactionsByStatus(params.Ctx, pool.TxStatusSelected)
	require.NoError(b, err)

	timeForSetup := time.Since(start)

	deployMetricsValues, err := metrics.GetValues(nil)
	allTxs, err := ParallelSendAndWait(
		authList,
		client,
		pl.GetTxsByStatus,
		params.NumberOfOperations,
		nil,
		nil,
		Sender,
	)
	require.NoError(b, err)

	err = transactions.WaitStatusSelected(pl.CountTransactionsByStatus, initialCount, params.NumberOfOperations)
	require.NoError(b, err)
	elapsed = time.Since(start)
	fmt.Printf("Total elapsed time: %s\n", elapsed)

	startMetrics := time.Now()
	var profilingResult string
	if profilingEnabled {
		profilingResult, err = metrics.FetchProfiling()
		require.NoError(b, err)
	}

	metrics.CalculateAndPrint(
		"eth",
		uint64(len(allTxs)),
		client,
		profilingResult,
		elapsed,
		deployMetricsValues,
		allTxs,
	)
	fmt.Printf("%s\n", profilingResult)
	timeForFetchAndPrintMetrics := time.Since(startMetrics)
	fmt.Printf("Time for setup: %s\n", timeForSetup)
	fmt.Printf("Time for fetching metrics: %s\n", timeForFetchAndPrintMetrics)
}
