# init-vault
This initialises the Hashicorp vault server on the manager nodes and unseals them automatically.

This is where security meets convenience, the keys are stored in a DynamoDB table. The DynamoDB table should be sufficiently protected at all times. In the future this may be updated to encrypt the data before storage in the DynamoDB table.

## Building the project
This project uses dep so it must be on your path to begin with.
```
dep ensure
go build
docker build -t init-vault .
```

## Running the container
The container can be run using the following command and passing environment variables where required.
```
docker run --restart=no -e DYNAMODB_TABLE=$DYNAMODB_TABLE  -e 'VAULT_SKIP_VERIFY=true' bhavikk/init-vault:latest
```
