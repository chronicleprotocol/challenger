package core

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/defiweb/go-eth/hexutil"
	"github.com/defiweb/go-eth/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockRpcClient struct {
	mock.Mock
}

func (m *mockRpcClient) Accounts(ctx context.Context) ([]types.Address, error) {
	args := m.Called(ctx)
	return args.Get(0).([]types.Address), args.Error(1)
}

func (m *mockRpcClient) BlockNumber(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *mockRpcClient) BlockByNumber(ctx context.Context, number types.BlockNumber, full bool) (*types.Block, error) {
	args := m.Called(ctx, number, full)
	return args.Get(0).(*types.Block), args.Error(1)
}

func (m *mockRpcClient) SendTransaction(ctx context.Context, tx *types.Transaction) (*types.Hash, *types.Transaction, error) {
	args := m.Called(ctx, tx)
	return args.Get(0).(*types.Hash), args.Get(1).(*types.Transaction), args.Error(2)
}

func (m *mockRpcClient) Call(ctx context.Context, call *types.Call, block types.BlockNumber) ([]byte, *types.Call, error) {
	args := m.Called(ctx, call, block)
	c := args.Get(1)
	if c == nil {
		return args.Get(0).([]byte), nil, args.Error(2)
	}
	return args.Get(0).([]byte), c.(*types.Call), args.Error(2)
}

func (m *mockRpcClient) GetLogs(ctx context.Context, query *types.FilterLogsQuery) ([]types.Log, error) {
	args := m.Called(ctx, query)
	return args.Get(0).([]types.Log), args.Error(1)
}

func (m *mockRpcClient) GetTransactionReceipt(ctx context.Context, hash types.Hash) (*types.TransactionReceipt, error) {
	args := m.Called(ctx, hash)
	return args.Get(0).(*types.TransactionReceipt), args.Error(1)
}

func TestGetFrom(t *testing.T) {
	mockRpcClient := new(mockRpcClient)
	provider := NewScribeOptimisticRPCProvider(mockRpcClient, nil)

	// gets zero address if no accounts
	call := mockRpcClient.On("Accounts", mock.Anything).Return([]types.Address{}, nil)
	addr := provider.GetFrom(context.TODO())
	assert.Equal(t, types.ZeroAddress, addr)
	mockRpcClient.AssertExpectations(t)
	call.Unset()

	// zero address on error
	call = mockRpcClient.On("Accounts", mock.Anything).Return([]types.Address{}, fmt.Errorf("error"))
	addr = provider.GetFrom(context.TODO())
	assert.Equal(t, types.ZeroAddress, addr)
	mockRpcClient.AssertExpectations(t)
	call.Unset()

	// gets first account
	call = mockRpcClient.On("Accounts", mock.Anything).Return([]types.Address{{0x1}}, nil)
	addr = provider.GetFrom(context.TODO())
	assert.Equal(t, types.Address{0x1}, addr)
	mockRpcClient.AssertExpectations(t)
	call.Unset()
}

func TestGetChallengePeriod(t *testing.T) {
	mockRpcClient := new(mockRpcClient)
	provider := NewScribeOptimisticRPCProvider(mockRpcClient, nil)
	address := types.MustAddressFromHex("0x1F7acDa376eF37EC371235a094113dF9Cb4EfEe1")

	// gets challenge period
	call := mockRpcClient.On("Call", mock.Anything, mock.Anything, types.LatestBlockNumber).
		Return(
			hexutil.MustHexToBytes("0x0000000000000000000000000000000000000000000000000000000000000257"),
			&types.Call{},
			nil,
		)
	period, err := provider.GetChallengePeriod(context.TODO(), address)
	assert.NoError(t, err)
	assert.Equal(t, uint16(599), period)
	mockRpcClient.AssertExpectations(t)
	call.Unset()

	// error on call
	call = mockRpcClient.On("Call", mock.Anything, mock.Anything, mock.Anything).
		Return([]byte{}, nil, fmt.Errorf("error"))
	period, err = provider.GetChallengePeriod(context.TODO(), address)
	assert.Error(t, err)
	assert.Equal(t, uint16(0), period)
	mockRpcClient.AssertExpectations(t)
	call.Unset()
}
