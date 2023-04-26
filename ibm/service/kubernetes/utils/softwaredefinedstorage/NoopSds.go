package softwaredefinedstorage

import (
	"log"
	"time"

	v2 "github.com/IBM-Cloud/bluemix-go/api/container/containerv2"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

// No Operation SDS Struct Defined
type NoopSds struct{}

func (noop NoopSds) PreWorkerReplace(worker v2.Worker, clusterConfig string, sdsTimeout time.Duration) error {
	log.Println("In Pre-Worker Replace - nothing done")
	return nil
}

func (noop NoopSds) PostWorkerReplace(worker v2.Worker, clusterConfig string, sdsTimeout time.Duration) error {
	log.Println("In Post-Worker Replace - nothing done")
	return nil
}
