package core

import (
	"context"
	"math/big"

	"github.com/defiweb/go-eth/types"
)

type RpcClient interface {
	Accounts(ctx context.Context) ([]types.Address, error)

	BlockNumber(ctx context.Context) (*big.Int, error)

	BlockByNumber(ctx context.Context, number types.BlockNumber, full bool) (*types.Block, error)

	SendTransaction(ctx context.Context, tx *types.Transaction) (*types.Hash, *types.Transaction, error)

	Call(ctx context.Context, call *types.Call, block types.BlockNumber) ([]byte, *types.Call, error)

	GetLogs(ctx context.Context, query *types.FilterLogsQuery) ([]types.Log, error)

	GetTransactionReceipt(ctx context.Context, hash types.Hash) (*types.TransactionReceipt, error)
}
