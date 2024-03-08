package eth_transfers

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygonHermez/zkevm-node/pool"
	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/params"
	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/transactions"
	"github.com/0xPolygonHermez/zkevm-node/test/contracts/bin/ERC20"
	"github.com/0xPolygonHermez/zkevm-node/test/operations"
	uniswap "github.com/0xPolygonHermez/zkevm-node/test/scripts/uniswap/pkg"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	gasLimit  = 21000
	ethAmount = big.NewInt(0)
	sleepTime = 1 * time.Second
	countTxs  = 0
)

// TxSender sends eth transfer to the sequencer
func Sender(l2Client *ethclient.Client, gasPrice *big.Int, auth *bind.TransactOpts,
	erc20SC *ERC20.ERC20, uniswapDeployments *uniswap.Deployments) ([]*types.Transaction, error) {
	return TxSender(l2Client, gasPrice, auth, &params.To, ethAmount, erc20SC, uniswapDeployments)
}

// TxSender sends eth transfer to the sequencer
func TxSender(l2Client *ethclient.Client, gasPrice *big.Int, auth *bind.TransactOpts, to *common.Address, value *big.Int,
	erc20SC *ERC20.ERC20, uniswapDeployments *uniswap.Deployments) ([]*types.Transaction, error) {
	senderNonce, err := l2Client.PendingNonceAt(params.Ctx, auth.From)
	if err != nil {
		panic(err)
	}

	fmt.Printf("sending tx num: %d, sender is: %s, sender nonce is: %d, receiver is: %s, value is: %v\n",
		countTxs+1, auth.From.String(), senderNonce, to.String(), value.String())

	tx := types.NewTx(&types.LegacyTx{
		GasPrice: gasPrice,
		Gas:      uint64(gasLimit),
		To:       to,
		Value:    value,
		Data:     nil,
		Nonce:    senderNonce,
	})

	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		return nil, err
	}

	err = l2Client.SendTransaction(params.Ctx, signedTx)
	for transactions.ShouldRetryError(err) {
		time.Sleep(sleepTime)
		err = l2Client.SendTransaction(params.Ctx, signedTx)
	}

	if err == nil {
		countTxs += 1
	}

	return []*types.Transaction{signedTx}, err
}

// ParallelSendAndWait sends a number of transactions and waits for them to be marked as pending in the pool
func ParallelSendAndWait(
	authList []*bind.TransactOpts,
	client *ethclient.Client,
	getTxsByStatus func(ctx context.Context, status pool.TxStatus, limit uint64) ([]pool.Transaction, error),
	nTxs uint64,
	erc20SC *ERC20.ERC20,
	uniswapDeployments *uniswap.Deployments,
	txSenderFunc func(l2Client *ethclient.Client, gasPrice *big.Int, auth *bind.TransactOpts, erc20SC *ERC20.ERC20,
		uniswapDeployments *uniswap.Deployments) ([]*types.Transaction, error),
) ([]*types.Transaction, error) {
	fmt.Printf("Sending %d txs ...\n", nTxs)

	gasPrice, err := client.SuggestGasPrice(params.Ctx)
	if err != nil {
		panic(err)
	}

	authLen := len(authList)
	allTxs := make([]*types.Transaction, 0, nTxs)
	for i := 0; i < int(nTxs); i++ {
		auth := authList[i%authLen]
		if auth == nil {
			continue
		}
		auth.GasLimit = 2100000
		auth.GasPrice = gasPrice

		txs, err := txSenderFunc(client, auth.GasPrice, auth, erc20SC, uniswapDeployments)
		if err != nil {
			return nil, err
		}
		allTxs = append(allTxs, txs...)
	}
	fmt.Println("All txs were sent!")

	fmt.Println("Waiting pending transactions To be added in the pool ...")
	err = operations.Poll(1*time.Second, params.DefaultDeadline, func() (bool, error) {
		// using a closure here To capture st and currentBatchNumber
		pendingTxs, err := getTxsByStatus(params.Ctx, pool.TxStatusPending, 0)
		if err != nil {
			panic(err)
		}

		pendingTxsCount := len(pendingTxs)

		//pendingTxsCount := 0
		//for _, tx := range pendingTxs {
		//	sender, err := state.GetSender(tx.Transaction)
		//	if err != nil {
		//		panic(err)
		//	}
		//	if sender == auth.From {
		//		pendingTxsCount++
		//	}
		//}

		fmt.Printf("amount of pending txs: %d\n\n", pendingTxsCount)
		done := pendingTxsCount <= 0
		return done, nil
	})
	if err != nil {
		return nil, err
	}

	fmt.Println("All pending txs are added in the pool!")

	return allTxs, nil
}
