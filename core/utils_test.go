package core

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/defiweb/go-eth/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func init() {
	// Speed up polling for tests.
	txConfirmationPollInterval = 10 * time.Millisecond
}

func TestWaitForTxConfirmation(t *testing.T) {
	hash := types.MustHashFromHex("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", types.PadNone)
	status := uint64(1)

	t.Run("nil client returns error", func(t *testing.T) {
		receipt, err := WaitForTxConfirmation(context.TODO(), nil, &hash, time.Second)
		assert.Nil(t, receipt)
		assert.ErrorContains(t, err, "ethereum client not set")
	})

	t.Run("nil txHash returns error", func(t *testing.T) {
		client := new(mockRpcClient)
		receipt, err := WaitForTxConfirmation(context.TODO(), client, nil, time.Second)
		assert.Nil(t, receipt)
		assert.ErrorContains(t, err, "tx hash is nil")
	})

	t.Run("timeout returns error", func(t *testing.T) {
		client := new(mockRpcClient)
		// Always return nil receipt to keep polling until timeout.
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return((*types.TransactionReceipt)(nil), nil)

		receipt, err := WaitForTxConfirmation(context.TODO(), client, &hash, 50*time.Millisecond)
		assert.Nil(t, receipt)
		assert.ErrorContains(t, err, "failed to wait for transaction confirmation")
	})

	t.Run("successful receipt returned", func(t *testing.T) {
		client := new(mockRpcClient)
		expected := &types.TransactionReceipt{
			TransactionHash: hash,
			Status:          &status,
			BlockHash:       types.MustHashFromHex("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", types.PadNone),
			BlockNumber:     big.NewInt(100),
		}
		client.On("GetTransactionReceipt", mock.Anything, hash).Return(expected, nil)

		receipt, err := WaitForTxConfirmation(context.TODO(), client, &hash, time.Second)
		require.NoError(t, err)
		assert.Equal(t, expected, receipt)
	})

	t.Run("transient error keeps polling until success", func(t *testing.T) {
		client := new(mockRpcClient)
		expected := &types.TransactionReceipt{
			TransactionHash: hash,
			Status:          &status,
			BlockHash:       types.MustHashFromHex("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", types.PadNone),
			BlockNumber:     big.NewInt(100),
		}
		// First call: error. Second call: success.
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return((*types.TransactionReceipt)(nil), fmt.Errorf("network error")).Once()
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return(expected, nil).Once()

		receipt, err := WaitForTxConfirmation(context.TODO(), client, &hash, time.Second)
		require.NoError(t, err)
		assert.Equal(t, expected, receipt)
		client.AssertNumberOfCalls(t, "GetTransactionReceipt", 2)
	})

	t.Run("nil receipt keeps polling until success", func(t *testing.T) {
		client := new(mockRpcClient)
		expected := &types.TransactionReceipt{
			TransactionHash: hash,
			Status:          &status,
			BlockHash:       types.MustHashFromHex("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", types.PadNone),
			BlockNumber:     big.NewInt(100),
		}
		// First call: nil receipt. Second call: success.
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return((*types.TransactionReceipt)(nil), nil).Once()
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return(expected, nil).Once()

		receipt, err := WaitForTxConfirmation(context.TODO(), client, &hash, time.Second)
		require.NoError(t, err)
		assert.Equal(t, expected, receipt)
		client.AssertNumberOfCalls(t, "GetTransactionReceipt", 2)
	})

	t.Run("receipt with nil status keeps polling", func(t *testing.T) {
		client := new(mockRpcClient)
		pending := &types.TransactionReceipt{
			TransactionHash: hash,
			Status:          nil,
		}
		confirmed := &types.TransactionReceipt{
			TransactionHash: hash,
			Status:          &status,
			BlockNumber:     big.NewInt(100),
		}
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return(pending, nil).Once()
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return(confirmed, nil).Once()

		receipt, err := WaitForTxConfirmation(context.TODO(), client, &hash, time.Second)
		require.NoError(t, err)
		assert.Equal(t, confirmed, receipt)
		client.AssertNumberOfCalls(t, "GetTransactionReceipt", 2)
	})

	t.Run("receipt with zero hash keeps polling", func(t *testing.T) {
		client := new(mockRpcClient)
		pending := &types.TransactionReceipt{
			TransactionHash: types.Hash{},
			Status:          &status,
		}
		confirmed := &types.TransactionReceipt{
			TransactionHash: hash,
			Status:          &status,
			BlockNumber:     big.NewInt(100),
		}
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return(pending, nil).Once()
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return(confirmed, nil).Once()

		receipt, err := WaitForTxConfirmation(context.TODO(), client, &hash, time.Second)
		require.NoError(t, err)
		assert.Equal(t, confirmed, receipt)
		client.AssertNumberOfCalls(t, "GetTransactionReceipt", 2)
	})

	t.Run("context cancellation returns error", func(t *testing.T) {
		client := new(mockRpcClient)
		client.On("GetTransactionReceipt", mock.Anything, hash).
			Return((*types.TransactionReceipt)(nil), nil)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(30 * time.Millisecond)
			cancel()
		}()

		receipt, err := WaitForTxConfirmation(ctx, client, &hash, 5*time.Second)
		assert.Nil(t, receipt)
		assert.ErrorContains(t, err, "failed to wait for transaction confirmation")
	})
}
