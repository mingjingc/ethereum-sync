package ethereum_sync

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type BlockInformation struct {
	Block    *types.Block
	Receipts types.Receipts
}

type EthereumSync struct {
	currentHeaderChan chan *types.Header // 当前区块链最高区块头
	subNewHeader      ethereum.Subscription
	syncNumber        uint64 // 已同步的最高区块号

	ethereumClient *ethclient.Client
	ctx            context.Context

	onSyncNewBlock func(information BlockInformation) error // 回调处理已同步的新区块信息

	stopCh          chan struct{}
	delaySyncNumber uint64 // 延迟几个块再同步，防止回滚可能
	gettingNewBlock int32  // 0: 当前没有再获取新区块信息； 1: 正在获取新区块信息
}

// ws://127.0.0.1:6688
func (s *EthereumSync) Run(websocketUrl string) (err error) {
	select {
	case <-s.stopCh:
		return errors.New("EthereumSync has stopped")
	default:
	}

	s.ethereumClient, err = ethclient.Dial(websocketUrl)
	if err != nil {
		return
	}
	s.subNewHeader, err = s.ethereumClient.SubscribeNewHead(s.ctx, s.currentHeaderChan)
	if err != nil {
		return err
	}

	return
}

func (s *EthereumSync) mainLoop() error {
	for {
		select {
		case newHeader := <-s.currentHeaderChan:
			for number := s.syncNumber + 1; number <= newHeader.Number.Uint64()-s.delaySyncNumber; number++ {
				if err := s.syncNewBlock(number); err != nil {
					s.handleStop()
					return err
				}
				s.syncNumber = number
			}
		case err := <-s.subNewHeader.Err():
			// TO DO STOP:
			s.handleStop()
			return err
		case <-s.stopCh:
			// TO DO STOP:
			s.handleStop()
			return nil
		}
	}
}

func (s *EthereumSync) syncNewBlock(number uint64) error {
	block, err := s.ethereumClient.BlockByNumber(s.ctx, big.NewInt(0).SetUint64(number))
	if err != nil {
		return err
	}
	receipts := make(types.Receipts, 0, block.Transactions().Len())
	for _, tx := range block.Transactions() {
		receipt, err := s.ethereumClient.TransactionReceipt(s.ctx, tx.Hash())
		if err != nil {
			return err
		}
		receipts = append(receipts, receipt)
	}
	info := BlockInformation{}
	info.Block = block
	info.Receipts = receipts
	return s.onSyncNewBlock(info)
}

func (s *EthereumSync) handleStop() {
	s.subNewHeader.Unsubscribe()
	s.ethereumClient.Close()
}

func (s *EthereumSync) Stop() {
	close(s.stopCh)
}
