package kubernetes

import (
	"log"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type Sds interface {
	preWorkerReplace() error
	postWorkerReplace() error
}

type odf struct{}

func (o odf) preWorkerReplace() error {

	log.Println("In preWorkerReplace for ODF")
	return nil

}

func (o odf) postWorkerReplace() error {

	log.Println("In postWorkerReplace for ODF")
	return nil

}
