package main

import (
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/hashicorp/vault/api"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	vaultTable := os.Getenv("DYNAMODB_TABLE")

	awsSession := session.Must(session.NewSession())
	identity := GetEC2IdentityDocument(awsSession)
	vaultClient := GetVaultClient()

	if !IsVaultInitialised(vaultClient) {
		initRequest := &api.InitRequest{SecretShares: 5, SecretThreshold: 3}
		initResponse, err := vaultClient.Sys().Init(initRequest)
		if err == nil {
			SaveInVaultTable(awsSession, vaultTable, identity.Region, initResponse)
		}
	}

	sealStatus := IsVaultSealed(vaultClient)
	if sealStatus.Sealed {
		keys := GetUnsealKeys(awsSession, vaultTable, identity.Region, sealStatus)
		UnsealVault(vaultClient, keys, sealStatus)
	}
	log.Info().Msg("Vault is unsealed")
}

func GetVaultClient() *api.Client {
	vaultClient, err := api.NewClient(nil)
	if err != nil {
		log.Error().Err(err).Msg("Unable to create new client")
		os.Exit(1)
	}
	return vaultClient
}

func IsVaultInitialised(client *api.Client) bool {
	// While the vault server isn't running, we wait
	for i := 0; i < 5; i++ {
		_, err := client.Sys().InitStatus()
		if err == nil {
			break
		}
		log.Info().Msg("Vault server still not running, waiting 30 seconds to try again")
		time.Sleep(30 * time.Second)
	}

	initialised, err := client.Sys().InitStatus()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get response")
		os.Exit(1)
	}
	log.Info().Msgf("Is vault currently initialised? %t", initialised)
	return initialised
}

func IsVaultSealed(client *api.Client) *api.SealStatusResponse {
	sealStatus, err := client.Sys().SealStatus()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get seal status")
		os.Exit(1)
	}
	log.Info().Msgf("Is vault currently sealed? %t", sealStatus.Sealed)
	return sealStatus
}

func UnsealVault(client *api.Client, keys []string, sealStatus *api.SealStatusResponse) {
	log.Info().Msg("Unsealing vault...")
	count := 0
	keyCount := len(keys)
	for sealStatus.Sealed && sealStatus.T >= sealStatus.Progress && count <= keyCount {
		progress, err := client.Sys().Unseal(keys[count])
		if err != nil {
			log.Error().Err(err).Msg("Error during unseal process")
		}
		sealStatus = progress
		count++
	}
}

func GetEC2IdentityDocument(session *session.Session) ec2metadata.EC2InstanceIdentityDocument {
	svc := ec2metadata.New(session)
	identity, err := svc.GetInstanceIdentityDocument()
	if err != nil {
		log.Error().Err(err).Msg("Unable to get instance metadata")
		os.Exit(1)
	}
	return identity
}

func SaveInVaultTable(session *session.Session, table, region string, secrets *api.InitResponse) {
	log.Info().Msgf("Saving data to DynamoDB table %s", table)
	svc := dynamodb.New(session, aws.NewConfig().WithRegion(region))
	_, err := svc.PutItem(CreatePutItemInput(table, "Root Token", secrets.RootToken))
	if err != nil {
		log.Error().Err(err)
	}

	for index, value := range secrets.KeysB64 {
		keyLabel := "Unseal Key " + strconv.Itoa(index+1)
		log.Info().Msgf("Saving %s", keyLabel)
		_, err := svc.PutItem(CreatePutItemInput(table, keyLabel, value))
		if err != nil {
			log.Error().Err(err)
		}
	}
	log.Info().Msg("Finished saving data to DynamoDB")
}

func CreatePutItemInput(table, key, value string) *dynamodb.PutItemInput {
	return &dynamodb.PutItemInput{
		TableName: aws.String(table),
		Item: map[string]*dynamodb.AttributeValue{
			"id": &dynamodb.AttributeValue{
				S: aws.String(key),
			},
			"value": &dynamodb.AttributeValue{
				S: aws.String(value),
			},
		},
	}
}

func GetUnsealKeys(session *session.Session, table, region string, sealStatus *api.SealStatusResponse) []string {
	log.Info().Msgf("Retrieving data from DynamoDB table %s", table)
	defer log.Info().Msg("Finished retrieving data from DynamoDB")

	keys := make([]string, sealStatus.N)
	svc := dynamodb.New(session, aws.NewConfig().WithRegion(region))

	for i := 0; i < sealStatus.N; i++ {
		keyLabel := "Unseal Key " + strconv.Itoa(i+1)
		input := &dynamodb.GetItemInput{
			TableName: aws.String(table),
			Key: map[string]*dynamodb.AttributeValue{
				"id": {
					S: aws.String(keyLabel),
				},
			},
		}
		result, err := svc.GetItem(input)
		if err != nil {
			log.Error().Err(err).Msg("Could not retrieve unseal key")
		}
		keys[i] = aws.StringValue(result.Item["value"].S)
	}
	return keys
}
