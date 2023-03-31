//go:build noswss
// +build noswss

package swsscommon

// This file contains stubs to work without linking to the swsscommon.
// Useful for quick compilation and testing on build servers which
// do not have swsscommon installed.

import (
	"encoding/json"
	"os"
)

// dbConfig holds the sonic database configs loaded from
// file /var/run/redis/sonic-db/database_config.json. This path can be
// customized through environemnt variable DB_CONFIG_PATH.
var dbConfig struct {
	Instances map[string]struct {
		Host string `json:"hostname"`
		Port int    `json:"port"`
		Sock string `json:"unix_socket_path"`
	} `json:"INSTANCES"`
	Databases map[string]struct {
		Id       int    `json:"id"`
		Sep      string `json:"separator"`
		Instance string `json:"instance"`
	} `json:"DATABASES"`
}

func SonicDBConfigIsGlobalInit() bool {
	return len(dbConfig.Databases) != 0
}

func SonicDBConfigInitializeGlobalConfig() error {
	f := "/var/run/redis/sonic-db/database_config.json"
	if override, ok := os.LookupEnv("DB_CONFIG_PATH"); ok {
		f = override
	}
	fp, err := os.Open(f)
	if err != nil {
		return err
	}
	defer fp.Close()
	jd := json.NewDecoder(fp)
	return jd.Decode(&dbConfig)
}

func SonicDBConfigIsInit() bool {
	return SonicDBConfigIsGlobalInit()
}

func SonicDBConfigInitialize() error {
	return SonicDBConfigInitializeGlobalConfig()
}

func SonicDBConfigReset() {
	dbConfig.Instances = nil
	dbConfig.Databases = nil
}

type vector[T any] []T

func (v vector[T]) Size() int   { return len(v) }
func (v vector[T]) Get(i int) T { return v[i] }
func (v vector[T]) Clear()      {}

type strVector = vector[string]

func DeleteVectorString(v strVector) {}

func SonicDBConfigGetNamespaces() strVector {
	return strVector{}
}

func SonicDBConfigGetDbId(dbName string, a ...any) int {
	if d, ok := dbConfig.Databases[dbName]; ok {
		return d.Id
	} else {
		return -1
	}
}

func SonicDBConfigGetSeparator(dbName string, a ...any) string {
	return dbConfig.Databases[dbName].Sep
}

func SonicDBConfigGetDbSock(dbName string, a ...any) string {
	instName := dbConfig.Databases[dbName].Instance
	return dbConfig.Instances[instName].Sock
}

func SonicDBConfigGetDbHostname(dbName string, a ...any) string {
	instName := dbConfig.Databases[dbName].Instance
	return dbConfig.Instances[instName].Host
}

func SonicDBConfigGetDbPort(dbName string, a ...any) int {
	instName := dbConfig.Databases[dbName].Instance
	return dbConfig.Instances[instName].Port
}

type SonicDBKey interface {
	GetNetns() string
	SetNetns(string)
	GetContainerName() string
	SetContainerName(string)
}

type dbkeyVetcor = vector[SonicDBKey]

func SonicDBConfigGetDbKeys() dbkeyVetcor {
	return dbkeyVetcor{}
}

func DeleteVectorSonicDbKey(v dbkeyVetcor) {}

func SonicDBConfigGetDbList(k ...any) strVector {
	return strVector{}
}

func NewSonicDBKey() SonicDBKey {
	return nil
}
