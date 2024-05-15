// Copyright  Grafana Labs
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/snowflakedb/gosnowflake"
	"github.com/youmark/pkcs8"
)

type Config struct {
	AccountName        string
	Username           string
	Password           string
	Role               string
	Warehouse          string
	PrivateKeyPath     string
	PrivateKeyPassword string
	PrivateKey         *rsa.PrivateKey
}

var (
	errNoAccountName = errors.New("account_name must be specified")
	errNoRole        = errors.New("role must be specified")
	errNoWarehouse   = errors.New("warehouse must be specified")
	errNoUsername    = errors.New("username must be specified")
	errNoAuth        = errors.New("password or private_key must be specified")
	errExclusiveAuth = errors.New("password and private_key are mutually exclusive and should not both be specified")
	errNoPrivKeyPwd  = errors.New("private_key needs a private_key_password to be specified")
	errDecodingPEM   = errors.New("error occurred while decoding private key PEM block")
)

func (c Config) Validate() error {
	if c.AccountName == "" {
		return errNoAccountName
	}

	if c.Username == "" {
		return errNoUsername
	}

	if c.Password == "" && c.PrivateKeyPath == "" {
		return errNoAuth
	}

	if c.Password != "" && c.PrivateKeyPath != "" {
		return errExclusiveAuth
	}

	// if c.PrivateKeyPath != "" && c.PrivateKeyPassword == "" {
	// 	return errNoPrivKeyPwd
	// }

	if c.Role == "" {
		return errNoRole
	}

	if c.Warehouse == "" {
		return errNoWarehouse
	}

	return nil
}

// decryptPrivateKey returns a RSA private key from the PrivateKeyPath and PrivateKeyPassword fields
// of the config.
// Assumes that the private key is encrypted in PKCS #8 syntax, as is recommended by Snowflake
func (c Config) decryptPrivateKey() (*rsa.PrivateKey, error) {
	var parsedPrivateKey *rsa.PrivateKey
	pk, err := os.ReadFile(c.PrivateKeyPath)
	if err != nil {
		fmt.Printf("Error opening file: %s", err)
		return nil, err
	}
	block, _ := pem.Decode(pk)
	if block == nil {
		return nil, errDecodingPEM
	}

	if c.PrivateKeyPassword != "" {
		// encrypted private key
		decryptedKey, err := pkcs8.ParsePKCS8PrivateKeyRSA(block.Bytes, []byte(c.PrivateKeyPassword))
		if err != nil {
			return nil, err
		}
		parsedPrivateKey = decryptedKey
	} else {
		// unencrypted private key
		unencryptedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		parsedPrivateKey = unencryptedKey.(*rsa.PrivateKey)
	}

	return parsedPrivateKey, nil
}

// snowflakeConnectionString returns a connection string to connect to the SNOWFLAKE database using the
// options specified in the config.
// Assumes the config is valid according to Validate().
func (c Config) snowflakeConnectionString() (string, error) {
	sf := &gosnowflake.Config{
		Account:   c.AccountName,
		User:      c.Username,
		Role:      c.Role,
		Warehouse: c.Warehouse,
		Database:  "SNOWFLAKE",
	}

	if c.PrivateKeyPath != "" {
		// key-pair authentication
		var pk, err = c.decryptPrivateKey()
		if err != nil {
			return "", err
		}
		sf.Authenticator = gosnowflake.AuthTypeJwt
		sf.PrivateKey = pk
		dsn, err := gosnowflake.DSN(sf)
		return dsn, err
	} else {
		// password authentication
		sf.Password = c.Password
		dsn, err := gosnowflake.DSN(sf)
		return dsn, err
	}

}
