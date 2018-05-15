/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package longhaul

import (
	"strconv"
	"sync"
	"testing"

	"time"

	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/suite"
)

// Rook Block Storage longhaul test
// Start Rook and set up a storage class and pool.
// Start multiple MySql databases that are using rook provisioned block storage.
// First block volume is persistent(mounted once) all the other volumes are mounted and unmounted per test
//NOTE: This tests doesn't clean up the cluster or volume after the run, the tests is designed
//to reuse the same cluster and volume for multiple runs or over a period of time.
// It is recommended to run this test with -count test param (to repeat th test n number of times)
// along with --load_parallel_runs params(number of concurrent operations per test) and
//--load_volumes(number of volumes that are created per test
func TestBlockLongHaul(t *testing.T) {
	suite.Run(t, new(BlockLongHaulSuite))
}

type BlockLongHaulSuite struct {
	suite.Suite
	kh        *utils.K8sHelper
	installer *installer.InstallHelper
	namespace string
	op        contracts.Setup
}

//Test set up - does the following in order
//create pool and storage class, create a PVC, Create a MySQL app/service that uses pvc
func (s *BlockLongHaulSuite) SetupSuite() {
	s.namespace = "longhaul-ns"
	s.op, s.kh, s.installer = StartBaseLoadTestOperations(s.T, s.namespace)
	createStorageClassAndPool(s.T, s.kh, s.namespace, "rook-block", "rook-pool")
}

//create a n number  ofPVC, Create a MySQL app/service that uses pvc
//The first PVC and mysql pod are persistent i.e. they are never deleted
//All other PVC and mounts are created and deleted/unmounted per test
func (s *BlockLongHaulSuite) TestBlockLonghaulRun() {
	var wg sync.WaitGroup
	wg.Add(s.installer.Env.LoadVolumeNumber)
	for i := 1; i <= s.installer.Env.LoadVolumeNumber; i++ {
		if i == 1 {
			go BlockVolumeOperations(s, &wg, "rook-block", "mysqlapp-persist", "mysqldb", "mysql-persist-claim", false)
		} else {
			go BlockVolumeOperations(s, &wg, "rook-block", "mysqlapp-ephemeral-"+strconv.Itoa(i), "mysqldbeph"+strconv.Itoa(i), "mysql-ephemeral-claim"+strconv.Itoa(i), randomBool())

		}

	}
	wg.Wait()
}

func BlockVolumeOperations(s *BlockLongHaulSuite, wg *sync.WaitGroup, storageClassName string, appName string, appLabel string, pvcName string, cleanup bool) {
	defer wg.Done()
	db := createPVCAndMountMysqlPod(s.T, s.kh, storageClassName, appName, appLabel, pvcName)
	performBlockOperations(s.installer, db)
	if cleanup {
		mySqlPodOperation(s.kh, storageClassName, appName, appLabel, pvcName, "delete")
		s.kh.WaitUntilPodWithLabelDeleted(appLabel, "default")
	}
	db.CloseConnection()
	db = nil
	time.Sleep(10 * time.Second)
}

func (s *BlockLongHaulSuite) TearDownSuite() {
	//clean up all ephemeral block storage, index 1 is persistent block storage which is going to be used among different test runs.
	for i := 2; i <= s.installer.Env.LoadVolumeNumber; i++ {
		mySqlPodOperation(s.kh, "rook-block", "mysqlapp-ephemeral-"+strconv.Itoa(i), "mysqldbeph"+strconv.Itoa(i), "mysql-ephemeral-claim"+strconv.Itoa(i), "delete")
	}
	s.kh = nil
	s = nil
}
