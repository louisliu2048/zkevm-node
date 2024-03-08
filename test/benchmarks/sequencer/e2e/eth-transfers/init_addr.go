package eth_transfers

import (
	"bufio"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"io"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/0xPolygonHermez/zkevm-node/pool"
	"github.com/0xPolygonHermez/zkevm-node/state"
	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/params"
	"github.com/0xPolygonHermez/zkevm-node/test/operations"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func initSender(client *ethclient.Client, adminAddr *bind.TransactOpts, targetList []*bind.TransactOpts, amount *big.Int,
	getTxsByStatus func(ctx context.Context, status pool.TxStatus, limit uint64) ([]pool.Transaction, error)) {
	adminAddr.GasLimit = 2100000
	fmt.Printf("Init %d accounts ...\n", len(targetList))
	if adminAddr.Nonce != nil {
		adminAddr.Nonce = nil
	}

	allTxs := make([]*types.Transaction, 0, len(targetList))
	for idx, auth := range targetList {
		if auth.Nonce != nil {
			auth.Nonce = nil
		}

		txs, err := TxSender(client, adminAddr.GasPrice, adminAddr, &auth.From, amount, nil, nil)
		if err != nil {
			fmt.Printf("fail to init account: %v, err is: %v\n", auth.From.String(), err)
			targetList[idx] = nil
		}
		allTxs = append(allTxs, txs...)
	}
	fmt.Println("All txs were sent!")

	fmt.Println("Waiting pending transactions To be added in the pool ...")
	err := operations.Poll(1*time.Second, params.DefaultDeadline, func() (bool, error) {
		// using a closure here To capture st and currentBatchNumber
		pendingTxs, err := getTxsByStatus(params.Ctx, pool.TxStatusPending, 0)
		if err != nil {
			panic(err)
		}
		pendingTxsCount := 0
		for _, tx := range pendingTxs {
			sender, err := state.GetSender(tx.Transaction)
			if err != nil {
				panic(err)
			}
			if sender == adminAddr.From {
				pendingTxsCount++
			}
		}

		fmt.Printf("amount of pending txs: %d\n\n", pendingTxsCount)
		done := pendingTxsCount <= 0
		return done, nil
	})
	if err != nil {
		fmt.Printf(" Fail to get tx from pool: %v\n", err)
	}
}

func loadSenderAddr(client *ethclient.Client, filepath string) []*bind.TransactOpts {
	gasPrice, err := client.SuggestGasPrice(params.Ctx)
	if err != nil {
		return nil
	}

	privateArray := ReadDataFromFile(filepath)
	AuthArray := make([]*bind.TransactOpts, 0, len(privateArray))
	for _, privateKey := range privateArray {
		key, _ := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))

		auth, err := bind.NewKeyedTransactorWithChainID(key, new(big.Int).SetUint64(params.ChainID))
		if err != nil {
			panic(err)
		}
		auth.GasPrice = gasPrice

		AuthArray = append(AuthArray, auth)
	}

	return AuthArray
}

func ReadDataFromFile(filepath string) []string {
	f, err := os.Open(filepath)
	if err != nil {
		panic(fmt.Errorf("failed to open file %s, error: %s\n", filepath, err.Error()))
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Println(fmt.Errorf("failed to close file %s, error: %s\n", filepath, err.Error()))
		}
	}(f)

	log.Printf("data is being loaded from path: %s, please wait\n", filepath)

	var lines []string
	count := 0
	rd := bufio.NewReader(f)
	for {
		privKey, err := rd.ReadString('\n')
		if err != nil || io.EOF == err {
			break
		}

		lines = append(lines, strings.TrimSpace(privKey))
		count++
	}

	log.Printf("%d records are loaded\n", count)
	return lines
}
