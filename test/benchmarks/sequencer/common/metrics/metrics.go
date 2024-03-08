package metrics

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	metricsLib "github.com/0xPolygonHermez/zkevm-node/metrics"
	"github.com/0xPolygonHermez/zkevm-node/sequencer/metrics"
	metricsState "github.com/0xPolygonHermez/zkevm-node/state/metrics"
	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/params"
	"github.com/0xPolygonHermez/zkevm-node/test/testutils"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	oneHundred    = 100
	profilingPort = 6060
)

// CalculateAndPrint calculates and prints the results
func CalculateAndPrint(
	txsType string,
	totalTxs uint64,
	client *ethclient.Client,
	profilingResult string,
	elapsed time.Duration,
	preTimeSub Values,
	allTxs []*types.Transaction,
) {
	fmt.Println("##########")
	fmt.Println("# Result #")
	fmt.Println("##########")
	fmt.Printf("Total time (including setup of environment and starting containers): %v\n", elapsed)
	totalTime := elapsed.Seconds()

	prometheusResp, err := FetchPrometheus()
	if err != nil {
		panic(fmt.Sprintf("error getting prometheus metrics: %v", err))
	}
	metricValues, err := GetValues(prometheusResp)
	if err != nil {
		panic(fmt.Sprintf("error getting prometheus metrics: %v\n", err))
	}

	getTxFromPoolTime := metricValues.GetTxFromPoolTime - preTimeSub.GetTxFromPoolTime
	fmt.Println("GetTxFromPoolTime is: ", getTxFromPoolTime)

	syncWaitL2BlockFinishedTime := metricValues.SyncWaitL2BlockFinishedTime - preTimeSub.SyncWaitL2BlockFinishedTime
	fmt.Println("SyncWaitL2BlockFinishedTime is: ", syncWaitL2BlockFinishedTime)

	syncWaitL2BlockStoreTime := metricValues.SyncWaitL2BlockStoreTime - preTimeSub.SyncWaitL2BlockStoreTime
	fmt.Println("SyncWaitL2BlockStoreTime is: ", syncWaitL2BlockStoreTime)

	execNewWIPL2BlockTime := metricValues.ExecNewWIPL2BlockTime - preTimeSub.ExecNewWIPL2BlockTime
	fmt.Println("ExecNewWIPL2BlockTime is: ", execNewWIPL2BlockTime)
	fmt.Println("CreateNewL2BlockCnt is: ", metricValues.CreateNewL2BlockCnt-preTimeSub.CreateNewL2BlockCnt)

	asyncExecL2BlockTime := metricValues.AsyncExecL2BlockTime - preTimeSub.AsyncExecL2BlockTime
	fmt.Println("AsyncExecL2BlockTime is: ", asyncExecL2BlockTime)
	fmt.Println("ExecL2BlockCnt is: ", metricValues.ExecL2BlockCnt-preTimeSub.ExecL2BlockCnt)

	actualTotalTime := metricValues.SequencerTotalProcessingTime - preTimeSub.SequencerTotalProcessingTime
	actualExecutorTime := metricValues.ExecutorTotalProcessingTime - preTimeSub.ExecutorTotalProcessingTime
	totalTime = actualTotalTime
	PrintSummary(txsType, params.NumberOfOperations, totalTxs, totalTime, actualExecutorTime, GetTotalGasUsedFromTxs(client, allTxs))

	if profilingResult != "" {
		fmt.Println("#####################")
		fmt.Println("# Profiling Metrics #")
		fmt.Println("#####################")
		fmt.Printf("%v", profilingResult)
	}
}

func PrintSummary(
	txsType string,
	totalTransactionsSent uint64,
	totalTxs uint64,
	processingTimeSequencer float64,
	processingTimeExecutor float64,
	totalGas uint64,
) {
	var transactionsTypes *string
	if txsType == "uniswap" {
		transactionsTypes, totalTransactionsSent = getTransactionsBreakdownForUniswap(totalTransactionsSent)
	}
	randomTxs := totalTxs - totalTransactionsSent
	txsType = strings.ToUpper(txsType)
	msg := fmt.Sprintf("# %s Benchmarks Summary #", txsType)
	delimiter := strings.Repeat("-", len(msg))
	fmt.Println(delimiter)
	fmt.Println(msg)
	fmt.Println(delimiter)

	if transactionsTypes != nil {
		fmt.Printf("Transactions Types: %s\n", *transactionsTypes)
	}
	fmt.Printf("Total Transactions: %d (%d predefined + %d random transactions)\n\n", totalTxs, totalTransactionsSent, randomTxs)
	fmt.Println("Processing Times:")
	fmt.Printf("- Total Processing Time: %.2f seconds\n", processingTimeSequencer)
	fmt.Printf("- Executor Processing Time: %.2f seconds\n", processingTimeExecutor)
	fmt.Printf("- Sequencer Processing Time: %.2f seconds\n\n", processingTimeSequencer-processingTimeExecutor)
	fmt.Println("Percentage Breakdown:")
	fmt.Printf("- Executor Time Percentage from Total: %.2f%%\n\n", (processingTimeExecutor/processingTimeSequencer)*oneHundred)
	fmt.Println("Metrics:")
	fmt.Printf("- Transactions per Second: %.2f\n", float64(totalTxs)/processingTimeSequencer)
	fmt.Printf("[the rest of the metrics are only for predefined transactions - excluding the random transactions]\n")
	fmt.Printf("- Gas per Second: %.2f\n", float64(totalGas)/processingTimeSequencer)
	fmt.Printf("- Total Gas Used: %d\n", totalGas)
	fmt.Printf("- Average Gas Used per Transaction: %d\n\n", totalGas/totalTxs)
}

func getTransactionsBreakdownForUniswap(numberOfOperations uint64) (*string, uint64) {
	transactionsBreakdown := fmt.Sprintf("Deployments, Approvals, Adding Liquidity, %d Swap Cycles (A -> B -> C)", numberOfOperations)
	totalTransactionsSent := (numberOfOperations * 2) + 17

	return &transactionsBreakdown, totalTransactionsSent
}

type Values struct {
	SequencerTotalProcessingTime float64
	AsyncExecL2BlockTime         float64
	ExecL2BlockCnt               float64
	SyncWaitL2BlockFinishedTime  float64
	SyncWaitL2BlockStoreTime     float64
	ExecNewWIPL2BlockTime        float64
	CreateNewL2BlockCnt          float64
	GetTxFromPoolTime            float64
	ExecutorTotalProcessingTime  float64
	WorkerTotalProcessingTime    float64
}

// GetValues gets the prometheus metric Values
func GetValues(metricsResponse *http.Response) (Values, error) {
	var err error
	if metricsResponse == nil {
		metricsResponse, err = FetchPrometheus()
		if err != nil {
			panic(fmt.Sprintf("error getting prometheus metrics: %v", err))
		}
	}

	mf, err := testutils.ParseMetricFamilies(metricsResponse.Body)
	if err != nil {
		return Values{}, err
	}
	sequencerTotalProcessingTimeHisto := mf[metrics.ProcessingTimeName].Metric[0].Histogram
	sequencerTotalProcessingTime := sequencerTotalProcessingTimeHisto.GetSampleSum()

	asyncExecL2BlockTimeNameHisto := mf[metrics.AsyncExecL2BlockTimeName].Metric[0].Histogram
	asyncExecL2BlockTime := asyncExecL2BlockTimeNameHisto.GetSampleSum()

	execL2BlockCntNameHisto := mf[metrics.ExecL2BlockCntName].Metric[0].Histogram
	execL2BlockCnt := execL2BlockCntNameHisto.GetSampleSum()

	execNewWIPL2BlockTimeNameHisto := mf[metrics.ExecNewWIPL2BlockTimeName].Metric[0].Histogram
	execNewWIPL2BlockTime := execNewWIPL2BlockTimeNameHisto.GetSampleSum()

	createNewL2BlockCntNameHisto := mf[metrics.CreateNewL2BlockCntName].Metric[0].Histogram
	createNewL2BlockCnt := createNewL2BlockCntNameHisto.GetSampleSum()

	syncWaitL2BlockStoreTimeHisto := mf[metrics.SyncWaitL2BlockStoreTimeName].Metric[0].Histogram
	syncWaitL2BlockStoreTime := syncWaitL2BlockStoreTimeHisto.GetSampleSum()

	getTxFromPoolTimeHisto := mf[metrics.GetTxFromPoolTimeName].Metric[0].Histogram
	getTxFromPoolTime := getTxFromPoolTimeHisto.GetSampleSum()

	SyncWaitL2BlockFinishedTimeNameHisto := mf[metrics.SyncWaitL2BlockFinishedTimeName].Metric[0].Histogram
	syncWaitL2BlockFinishedTime := SyncWaitL2BlockFinishedTimeNameHisto.GetSampleSum()

	workerTotalProcessingTimeHisto := mf[metrics.WorkerProcessingTimeName].Metric[0].Histogram
	workerTotalProcessingTime := workerTotalProcessingTimeHisto.GetSampleSum()

	executorTotalProcessingTimeHisto := mf[metricsState.ExecutorProcessingTimeName].Metric[0].Histogram
	executorTotalProcessingTime := executorTotalProcessingTimeHisto.GetSampleSum()

	return Values{
		SequencerTotalProcessingTime: sequencerTotalProcessingTime,
		AsyncExecL2BlockTime:         asyncExecL2BlockTime,
		ExecL2BlockCnt:               execL2BlockCnt,
		GetTxFromPoolTime:            getTxFromPoolTime,
		SyncWaitL2BlockFinishedTime:  syncWaitL2BlockFinishedTime,
		SyncWaitL2BlockStoreTime:     syncWaitL2BlockStoreTime,
		ExecNewWIPL2BlockTime:        execNewWIPL2BlockTime,
		CreateNewL2BlockCnt:          createNewL2BlockCnt,
		ExecutorTotalProcessingTime:  executorTotalProcessingTime,
		WorkerTotalProcessingTime:    workerTotalProcessingTime,
	}, nil
}

// FetchPrometheus fetches the prometheus metrics
func FetchPrometheus() (*http.Response, error) {
	fmt.Printf("Fetching prometheus metrics ...\n")
	return http.Get(fmt.Sprintf("http://localhost:%d%s", params.PrometheusPort, metricsLib.Endpoint))
}

// FetchProfiling fetches the profiling metrics
func FetchProfiling() (string, error) {
	fullUrl := fmt.Sprintf("http://localhost:%d%s", profilingPort, metricsLib.ProfileEndpoint)
	fmt.Printf("Fetching profiling metrics from: %s ...", fullUrl)
	cmd := exec.Command("go", "tool", "pprof", "-show=sequencer", "-top", fullUrl)
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("error fetching profiling metrics: %v", err))
	}
	return string(out), err
}

func PrintUniswapDeployments(deployments time.Duration, count uint64) {
	fmt.Println("#######################")
	fmt.Println("# Uniswap Deployments #")
	fmt.Println("#######################")
	fmt.Printf("Total time took for the sequencer to deploy all contracts: %v\n", deployments)
	fmt.Printf("Number of txs sent: %d\n", count)
}

// GetTotalGasUsedFromTxs sums the total gas used from the transactions
func GetTotalGasUsedFromTxs(client *ethclient.Client, txs []*types.Transaction) uint64 {
	// calculate the total gas used
	var totalGas uint64
	for _, tx := range txs {
		// Fetch the transaction receipt
		receipt, err := client.TransactionReceipt(params.Ctx, tx.Hash())
		if err != nil {
			fmt.Println("Unable to fetch transaction receipt", "error", err)
			continue
		}

		totalGas += receipt.GasUsed

		if receipt.Status != types.ReceiptStatusSuccessful {
			reason := "unknown"
			if receipt.Status == types.ReceiptStatusFailed {
				reason = "reverted"
			}
			fmt.Println("Transaction failed", "tx", tx.Hash(), "status", receipt.Status, "reason", reason)
			continue
		}
	}

	return totalGas
}
