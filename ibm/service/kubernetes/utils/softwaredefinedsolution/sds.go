package sds

import (
	v2 "github.com/IBM-Cloud/bluemix-go/api/container/containerv2"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

// Common Interface for different Software Defined Solutions
type Sds interface {
	PreWorkerReplace(worker v2.Worker, clusterConfig string) error
	PostWorkerReplace(worker v2.Worker, clusterConfig string) error
}

// ODF Struct Defined
type Odf struct{}
