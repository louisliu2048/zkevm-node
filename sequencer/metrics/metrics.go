package metrics

import (
	"time"

	"github.com/0xPolygonHermez/zkevm-node/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Prefix for the metrics of the sequencer package.
	Prefix = "sequencer_"
	// SequencesSentToL1CountName is the name of the metric that counts the sequences sent to L1.
	SequencesSentToL1CountName = Prefix + "sequences_sent_to_L1_count"
	// GasPriceEstimatedAverageName is the name of the metric that shows the average estimated gas price.
	GasPriceEstimatedAverageName = Prefix + "gas_price_estimated_average"
	// TxProcessedName is the name of the metric that counts the processed transactions.
	TxProcessedName = Prefix + "transaction_processed"
	// SequencesOversizedDataErrorName is the name of the metric that counts the sequences with oversized data error.
	SequencesOversizedDataErrorName = Prefix + "sequences_oversized_data_error"
	// EthToPolPriceName is the name of the metric that shows the Ethereum to Pol price.
	EthToPolPriceName = Prefix + "eth_to_pol_price"
	// SequenceRewardInPolName is the name of the metric that shows the reward in Pol of a sequence.
	SequenceRewardInPolName = Prefix + "sequence_reward_in_pol"
	// ProcessingTimeName is the name of the metric that shows the processing time.
	ProcessingTimeName               = Prefix + "processing_time"
	BeginBlockTimeName               = Prefix + "begin_block_time"
	EndBlockTimeName                 = Prefix + "end_block_time"
	L2BlockExecTimeName              = Prefix + "l2_block_exec_time"
	L2BlockStoreTimeName             = Prefix + "l2_block_store_time"
	GetTxFromPoolTimeName            = Prefix + "get_tx_from_pool"
	FinalizeL2BlockAtBeginTimeName   = Prefix + "finalize_l2_block_at_begin_time"
	CloseWIPL2BlockAtBeginTimeName   = Prefix + "close_wip_l2Block_time"
	OpenNewWIPL2BlockAtBeginTimeName = Prefix + "open_new_wip_l2Block_time"

	// WorkerPrefix is the prefix for the metrics of the worker.
	WorkerPrefix = Prefix + "worker_"
	// WorkerProcessingTimeName is the name of the metric that shows the worker processing time.
	WorkerProcessingTimeName = WorkerPrefix + "processing_time"
	// TxProcessedLabelName is the name of the label for the processed transactions.
	TxProcessedLabelName = "status"
)

// TxProcessedLabel represents the possible values for the
// `sequencer_transaction_processed` metric `type` label.
type TxProcessedLabel string

const (
	// TxProcessedLabelSuccessful represents a successful transaction
	TxProcessedLabelSuccessful TxProcessedLabel = "successful"
	// TxProcessedLabelInvalid represents an invalid transaction
	TxProcessedLabelInvalid TxProcessedLabel = "invalid"
	// TxProcessedLabelFailed represents a failed transaction
	TxProcessedLabelFailed TxProcessedLabel = "failed"
)

// Register the metrics for the sequencer package.
func Register() {
	var (
		counters    []prometheus.CounterOpts
		counterVecs []metrics.CounterVecOpts
		gauges      []prometheus.GaugeOpts
		histograms  []prometheus.HistogramOpts
	)

	counters = []prometheus.CounterOpts{
		{
			Name: SequencesSentToL1CountName,
			Help: "[SEQUENCER] total count of sequences sent to L1",
		},
		{
			Name: SequencesOversizedDataErrorName,
			Help: "[SEQUENCER] total count of sequences with oversized data error",
		},
	}

	counterVecs = []metrics.CounterVecOpts{
		{
			CounterOpts: prometheus.CounterOpts{
				Name: TxProcessedName,
				Help: "[SEQUENCER] number of transactions processed",
			},
			Labels: []string{TxProcessedLabelName},
		},
	}

	gauges = []prometheus.GaugeOpts{
		{
			Name: GasPriceEstimatedAverageName,
			Help: "[SEQUENCER] average gas price estimated",
		},
		{
			Name: EthToPolPriceName,
			Help: "[SEQUENCER] eth to pol price",
		},
		{
			Name: SequenceRewardInPolName,
			Help: "[SEQUENCER] reward for a sequence in pol",
		},
	}

	histograms = []prometheus.HistogramOpts{
		{
			Name: ProcessingTimeName,
			Help: "[SEQUENCER] processing time",
		},
		{
			Name: BeginBlockTimeName,
			Help: "[SEQUENCER] begin block time",
		},
		{
			Name: EndBlockTimeName,
			Help: "[SEQUENCER] end block time",
		},
		{
			Name: L2BlockExecTimeName,
			Help: "[SEQUENCER] l2 block exec time",
		},
		{
			Name: L2BlockStoreTimeName,
			Help: "[SEQUENCER] l2 block store time",
		},
		{
			Name: GetTxFromPoolTimeName,
			Help: "[SEQUENCER] get tx from pool time",
		},
		{
			Name: FinalizeL2BlockAtBeginTimeName,
			Help: "[SEQUENCER] finalize l2 block at begin time",
		},
		{
			Name: CloseWIPL2BlockAtBeginTimeName,
			Help: "[SEQUENCER] close wip l2 block at begin time",
		},
		{
			Name: OpenNewWIPL2BlockAtBeginTimeName,
			Help: "[SEQUENCER] open new wip l2 block at begin time",
		},
		{
			Name: WorkerProcessingTimeName,
			Help: "[SEQUENCER] worker processing time",
		},
	}

	metrics.RegisterCounters(counters...)
	metrics.RegisterCounterVecs(counterVecs...)
	metrics.RegisterGauges(gauges...)
	metrics.RegisterHistograms(histograms...)
}

// AverageGasPrice sets the gauge to the given average gas price.
func AverageGasPrice(price float64) {
	metrics.GaugeSet(GasPriceEstimatedAverageName, price)
}

// SequencesSentToL1 increases the counter by the provided number of sequences
// sent to L1.
func SequencesSentToL1(numSequences float64) {
	metrics.CounterAdd(SequencesSentToL1CountName, numSequences)
}

// TxProcessed increases the counter vector by the provided transactions count
// and for the given label (status).
func TxProcessed(status TxProcessedLabel, count float64) {
	metrics.CounterVecAdd(TxProcessedName, string(status), count)
}

// SequencesOvesizedDataError increases the counter for sequences that
// encounter a OversizedData error.
func SequencesOvesizedDataError() {
	metrics.CounterInc(SequencesOversizedDataErrorName)
}

// EthToPolPrice sets the gauge for the Ethereum to Pol price.
func EthToPolPrice(price float64) {
	metrics.GaugeSet(EthToPolPriceName, price)
}

// SequenceRewardInPol sets the gauge for the reward in Pol of a sequence.
func SequenceRewardInPol(reward float64) {
	metrics.GaugeSet(SequenceRewardInPolName, reward)
}

// ProcessingTime observes the last processing time on the histogram.
func ProcessingTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(ProcessingTimeName, execTimeInSeconds)
}

func BeginBlockTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(BeginBlockTimeName, execTimeInSeconds)
}

func EndBlockTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(EndBlockTimeName, execTimeInSeconds)
}

func L2BlockExecTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(L2BlockExecTimeName, execTimeInSeconds)
}

func L2BlockStoreTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(L2BlockStoreTimeName, execTimeInSeconds)
}

func GetTxFromPoolTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(GetTxFromPoolTimeName, execTimeInSeconds)
}

func FinalizeL2BlockAtBeginTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(FinalizeL2BlockAtBeginTimeName, execTimeInSeconds)
}

func CloseWIPL2BlockAtBeginTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(CloseWIPL2BlockAtBeginTimeName, execTimeInSeconds)
}

func OpenNewWIPL2BlockAtBeginTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(OpenNewWIPL2BlockAtBeginTimeName, execTimeInSeconds)
}

// WorkerProcessingTime observes the last processing time on the histogram.
func WorkerProcessingTime(lastProcessTime time.Duration) {
	execTimeInSeconds := float64(lastProcessTime) / float64(time.Second)
	metrics.HistogramObserve(WorkerProcessingTimeName, execTimeInSeconds)
}
