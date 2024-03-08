package eth_transfers

import (
	"bufio"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"io"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/0xPolygonHermez/zkevm-node/test/benchmarks/sequencer/common/params"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
)

func initSender(client *ethclient.Client, adminAddr *bind.TransactOpts, targetList []*bind.TransactOpts, amount *big.Int) {
	for _, auth := range targetList {
		if _, err := TxSender(client, adminAddr.GasPrice, adminAddr, &auth.From, amount, nil, nil); err != nil {
			fmt.Printf("Fail to send eth to target addr: %v, err is: %v\n", auth.From.String(), err)
		}
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
