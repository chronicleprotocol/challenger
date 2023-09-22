# MockScribeOptimistic

This is a dummy contract that provides all the interfaces expected from `ScribeOptimistic` but requires no actual signing. Useful for dev.

To deploy, make sure you're on the **Sepolia** network and do

```
forge create src/MockScribeOptimistic.sol:MockScribeOptimistic
```

The script `contract/test.sh` will give you a means to configure the mock to provide testing behaviors.

NOTE there is an existing contract on Sepolia that is available and can be used for dev:

[0x1Fb90D9bB6207f9Df404beE3612c3c597B2E65bb](https://sepolia.etherscan.io/address/0x1Fb90D9bB6207f9Df404beE3612c3c597B2E65bb)