package core

import (
	"context"
	"fmt"
	"time"

	"github.com/defiweb/go-eth/types"
	logger "github.com/sirupsen/logrus"
)

// WaitForTxConfirmation waits for the transaction to be confirmed.
func WaitForTxConfirmation(
	ctx context.Context,
	client RpcClient,
	txHash *types.Hash,
	timeout time.Duration,
) (*types.TransactionReceipt, error) {
	if client == nil {
		return nil, fmt.Errorf("ethereum client not set")
	}
	if txHash == nil {
		return nil, fmt.Errorf("tx hash is nil")
	}

	// check +- every block
	ticker := time.NewTicker(12 * time.Second)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to wait for transaction confirmation")
		case <-ticker.C:
			logger.WithField("txHash", txHash).Tracef("checking transaction confirmation")

			receipt, err := client.GetTransactionReceipt(ctx, *txHash)
			if err != nil {
				logger.WithField("txHash", txHash).Errorf("failed to get transaction receipt: %v", err)
				continue
			}
			if receipt == nil {
				continue
			}

			if receipt.Status == nil || receipt.TransactionHash.IsZero() {
				logger.WithField("txHash", txHash).Tracef("transaction is not yet confirmed")
				continue
			}
			return receipt, nil
		}
	}
}
