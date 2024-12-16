# Challenger GoLang version

Challenger searches for `opPoked` events for `ScribeOptimistic` contract. It verifies the poked Schnorr signature and challenges if it's invalid.

```bash
Usage:
  run [flags]

Aliases:
  run, agent

Flags:
  -a, --addresses 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f   ScribeOptimistic contract address. Example: 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f
      --chain-id uint                                          If no chain_id provided binary will try to get chain_id from given RPC
      --from-block uint                                        Block number to start from. If not provided, binary will try to get it from given RPC
  -h, --help                                                   help for run
      --keystore string                                        Keystore file (NOT FOLDER), path to key .json file. If provided, no need to use --secret-key
      --password string                                        Key raw password as text
      --password-file string                                   Path to key password file
      --rpc-url string                                         Node HTTP RPC_URL, normally starts with https://****
      --secret-key 0x******                                    Private key in format 0x****** or `*******`. If provided, no need to use --keystore
      --subscription-url string                                [Optional] Used if you want to subscribe to events rather than poll, typically starts with wss://****
      --tx-type legacy                                         Transaction type definition, possible values are: legacy, `eip1559` or `none` (default "none")
```

Note that in *all* cases you must provide `--rpc-url`, but if you want to use event driven listening instead of polling you also need to provide `--subscription-url`.

## Example

Starting with private key

```bash
challenger run --addresses 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f --rpc-url http://localhost:3334 --secret-key 0x******
```

Starting with key file and password

```bash
challenger run -a 0x891E368fE81cBa2aC6F6cc4b98e684c106e2EF4f --rpc-url http://localhost:3334 --keystore /path/to/key.json --password-file /path/to/file
```

## Using Docker image

We provide a Docker image for the Challenger GoLang version. 
You can use it to run the Challenger without installing GoLang on your machine.

[Docker image](https://github.com/chronicleprotocol/challenger/pkgs/container/challenger-go)

```bash 
docker pull ghcr.io/chronicleprotocol/challenger-go:latest
```

To run docker image you can use the following command:

```bash
docker run -d ghcr.io/chronicleprotocol/challenger-go:latest run -a ADDRESS1 -a ADDRESS2 -a ADDRESS3 --rpc-url http://localhost:3334 --secret-key asdfasdfas --tx-type legacy 
```

For keystore usage:

```bash
docker run -d ghcr.io/chronicleprotocol/challenger-go:latest run -a ADDRESS1 -a ADDRESS2 -a ADDRESS3 --rpc-url http://localhost:3334 --keystore /keystore/keystore.json --password-file /password/password.txt --chain-id 1 --tx-type legacy
```

## Prometheus metrics

By default, Challenger exposes Prometheus metrics on port `9090`.
You can have access to the metrics by visiting `http://localhost:9090/metrics` in your browser or route it from docker.

```bash
docker run -d -p 9090:9090 ghcr.io/chronicleprotocol/challenger-go:latest run -a ADDRESS1 -a ADDRESS2 -a ADDRESS3 --rpc-url http://localhost:3334 --secret-key asdfasdfas --tx-type legacy 
```

## Building docker image

SERVER_VERSION have to be same as release but without `v`, if release is `v0.0.10` then `SERVER_VERSION=0.0.10`

```bash
docker build -t challenger-go .
```

Usage:

```bash
docker run --rm challenger-go
```

Full example:

```bash
docker run --it --rm --name challenger-go run -a ADDRESS --rpc-url http://localhost:3334 --secret-key asdfasdfas
```