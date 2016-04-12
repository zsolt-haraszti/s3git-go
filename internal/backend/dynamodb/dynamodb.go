/*
 * Copyright 2016 Frank Wessels <fwessels@xs4all.nl>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dynamodb

import (
	"encoding/hex"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/s3git/s3git-go/internal/config"
	"io"
	"io/ioutil"
)

type Client struct {
	Table     string
	Region    string
	AccessKey string
	SecretKey string
}

const KEY_NAME = "K"
const VAL_NAME = "V"

func MakeClient(remote config.RemoteObject) *Client {

	// TODO: Check that table exists, if not create

	return &Client{
		Table:     remote.DynamoDbTable,
		Region:    remote.DynamoDbRegion,
		AccessKey: remote.DynamoDbAccessKey, SecretKey: remote.DynamoDbSecretKey}
}

// Upload a chunk to DynamoDB
func (c *Client) UploadWithReader(hash string, r io.Reader) error {

	svc := dynamodb.New(session.New(c.getAwsConfig()))

	hx, _ := hex.DecodeString(hash)

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	item := make(map[string]*dynamodb.AttributeValue)
	item[KEY_NAME] = &dynamodb.AttributeValue{B: hx}
	item[VAL_NAME] = &dynamodb.AttributeValue{B: b}

	_, err = svc.PutItem(&dynamodb.PutItemInput{
		Item:      item,
		TableName: aws.String(c.Table),
	})
	if err != nil {
		return err
	}

	return nil
}

// Verify the existence of a hash in DynamoDB
func (c *Client) VerifyHash(hash string) (bool, error) {

	svc := dynamodb.New(session.New(c.getAwsConfig()))

	hx, _ := hex.DecodeString(hash)

	item := make(map[string]*dynamodb.AttributeValue)
	item[KEY_NAME] = &dynamodb.AttributeValue{B: hx}

	result, err := svc.GetItem(&dynamodb.GetItemInput{
		Key:       item,
		TableName: aws.String(c.Table),
		AttributesToGet: []*string{
			aws.String(KEY_NAME), // Just ask for key back
		},
	})
	if err != nil {
		return false, err
	}

	verified := len(result.Item) == 1

	return verified, nil
}

// Download a chunk from DynamoDB
func (c *Client) DownloadWithWriter(hash string, w io.WriterAt) error {

	svc := dynamodb.New(session.New(c.getAwsConfig()))

	hx, _ := hex.DecodeString(hash)

	item := make(map[string]*dynamodb.AttributeValue)
	item[KEY_NAME] = &dynamodb.AttributeValue{B: hx}

	result, err := svc.GetItem(&dynamodb.GetItemInput{
		Key:       item,
		TableName: aws.String(c.Table),
	})
	if err != nil {
		return err
	}

	_, err = w.WriteAt(result.Item[VAL_NAME].B, 0)
	if err != nil {
		return err
	}

	return nil
}

// List with a prefix string in DynamoDB
func (c *Client) List(prefix string, action func(key string)) ([]string, error) {

	/*go*/ func(prefix string, action func(key string), cfg *aws.Config) {

		svc := dynamodb.New(session.New(cfg))

		hx, _ := hex.DecodeString(prefix)

		params := &dynamodb.ScanInput{
			TableName: aws.String(c.Table),
			AttributesToGet: []*string{
				aws.String(KEY_NAME),
			},
			ScanFilter: map[string]*dynamodb.Condition{
				KEY_NAME: {
					ComparisonOperator: aws.String("BEGINS_WITH"),
					AttributeValueList: []*dynamodb.AttributeValue{
						{
							B: hx,
						},
					},
				},
			},
			ReturnConsumedCapacity: aws.String("TOTAL"),
		}
		resp, err := svc.Scan(params)
		if err != nil {
			return
		}
		for _, k := range resp.Items {
			action(hex.EncodeToString(k[KEY_NAME].B))
		}
	}(prefix, action, c.getAwsConfig())

	result := make([]string, 0)

	return result, nil
}

func (c *Client) getAwsConfig() *aws.Config {

	s3Config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(c.AccessKey, c.SecretKey, ""),
		Region:      aws.String(c.Region)}

	return s3Config
}