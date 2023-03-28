package kubernetes

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	v2 "github.com/IBM-Cloud/bluemix-go/api/container/containerv2"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/clientcmd"
)

type Sds interface {
	preWorkerReplace(worker v2.Worker, cluster_config string) error
	postWorkerReplace(worker v2.Worker) error
}

type odf struct{}

func (o odf) preWorkerReplace(worker v2.Worker, cluster_config string) error {

	log.Println("\nInside preWorkerReplace for ODF")

	//1. Load the cluster config
	log.Println("\nThis is the cluster config\n", cluster_config)

	config, err := clientcmd.BuildConfigFromFlags("", cluster_config)

	if err != nil {
		return fmt.Errorf("[ERROR] Invalid kubeconfig, failed to set context: %s", err)
	}

	log.Println("\nConfig Made")

	//2. create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("[ERROR] Invalid kubeconfig,, failed to create clientset: %s", err)
	}

	// return nil
	workerName := worker.NetworkInterfaces[0].IpAddress
	log.Println("This is the Worker Name", workerName)

	// Mon Pods
	err = scaleDownPods(clientset, workerName, "app", "rook-ceph-mon", "mon")
	if err != nil {
		return fmt.Errorf("[ERROR] Error in Scaling")
	}
	// OSD Pods
	err = scaleDownPods(clientset, workerName, "app", "rook-ceph-osd", "ceph-osd-id")
	if err != nil {
		return fmt.Errorf("[ERROR] Error in Scaling")
	}
	// Crash Collector
	err = scaleDownPods(clientset, workerName, "app", "rook-ceph-crashcollector", "node_name")
	if err != nil {
		return fmt.Errorf("[ERROR] Error in Scaling")
	}

	node := workerName
	n, err := clientset.CoreV1().Nodes().Get(context.Background(), node, metav1.GetOptions{})

	if err != nil {
		return fmt.Errorf("Node %s not found\n", node)
	}

	time.Sleep(time.Second * 45)

	// Cordon the node
	n.Spec.Unschedulable = true
	_, err = clientset.CoreV1().Nodes().Update(context.Background(), n, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Unable to cordon node %s\n", node)
	}

	time.Sleep(time.Second * 45)

	//Drain the node
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + workerName})

	if err != nil {
		return fmt.Errorf("[ERROR] Invalid kubeconfig, failed to list resource: %s", err)
	}

	for _, pod := range pods.Items {
		EvictPod(clientset, pod.Name, pod.Namespace)
		fmt.Printf("Pod %s has been evicted\n", pod.Name)
	}

	fmt.Printf("Node %s has been drained\n", node)

	return nil

}

func EvictPod(client *kubernetes.Clientset, name, namespace string) error {

	return client.PolicyV1beta1().Evictions(namespace).Evict(context.TODO(), &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace},
	})
}

func scaleDownPods(clientset *kubernetes.Clientset, workerName string, appLabel string, appLabelValue string, idLabel string) error {

	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{appLabel: appLabelValue}}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
		FieldSelector: "spec.nodeName=" + workerName}

	pods, err := clientset.CoreV1().Pods("openshift-storage").List(context.TODO(), listOptions)

	if err != nil {
		return fmt.Errorf("[ERROR] Invalid kubeconfig, failed to list resource: %s", err)
	}

	if len(pods.Items) == 0 {
		return nil
	}

	for _, pod := range pods.Items {
		fmt.Printf("Pod name: %v\n", pod.Name)
		deploymentName := pod.Labels[appLabel] + "-" + pod.Labels[idLabel]
		deploymentsClient := clientset.AppsV1().Deployments("openshift-storage")

		if err != nil {
			log.Fatal(err)
		}

		result, getErr := deploymentsClient.Get(context.TODO(), deploymentName, metav1.GetOptions{})

		if getErr != nil {
			return fmt.Errorf("failed to get latest version of deployment: %v", getErr)
		}

		result.Spec.Replicas = int32Ptr(0) // reduce replica count
		scaleDeployment, err := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})

		if err != nil {
			fmt.Printf("error updating scale object: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully scaled deployment %s to %d replicas", deploymentName, *scaleDeployment.Spec.Replicas)
	}
	return nil
}

func (o odf) postWorkerReplace(worker v2.Worker) error {

	log.Println("In postWorkerReplace for ODF")

	return nil

}

func int32Ptr(i int32) *int32 { return &i }

// // Mon Pod

// labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"app": "rook-ceph-mon"}}
// listOptions := metav1.ListOptions{
// 	LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
// 	FieldSelector: "spec.nodeName=" + "10.248.0.99"}
// pods, err := clientset.CoreV1().Pods("openshift-storage").List(context.TODO(), listOptions)

// // pods, err := clientset.CoreV1().Pods("openshift-storage").List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + worker.NetworkInterfaces[0].IpAddress})
// if err != nil {
// 	return fmt.Errorf("[ERROR] Invalid kubeconfig, failed to list resource: %s", err)

// }

// if err != nil {
// 	panic(err.Error())
// }

// log.Println("This is the Worker Name", worker.NetworkInterfaces[0].IpAddress)

// log.Println("\nIn preWorkerReplace for ODF")
// for _, pod := range pods.Items {
// 	fmt.Printf("Pod name: %v\n", pod.Name)
// 	deploymentName := pod.Labels["app"] + "-" + pod.Labels["mon"]
// 	s, err := clientset.AppsV1().Deployments("openshift-storage").GetScale(context.TODO(), deploymentName, metav1.GetOptions{})
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	sc := *s
// 	sc.Spec.Replicas = 0
// 	scaleDeployment, err := clientset.AppsV1().Deployments("openshift-storage").UpdateScale(context.TODO(), deploymentName, &sc, metav1.UpdateOptions{})
// 	if err != nil {
// 		fmt.Printf("error updating scale object: %v\n", err)
// 		os.Exit(1)
// 	}
// 	fmt.Printf("Successfully scaled deployment %s to %d replicas", deploymentName, scaleDeployment.Spec.Replicas)
// }

// // Osd Pod

// labelSelector = metav1.LabelSelector{MatchLabels: map[string]string{"app": "rook-ceph-osd"}}
// listOptions = metav1.ListOptions{
// 	LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
// 	FieldSelector: "spec.nodeName=" + "10.248.0.99"}
// pods, err = clientset.CoreV1().Pods("openshift-storage").List(context.TODO(), listOptions)

// // pods, err := clientset.CoreV1().Pods("openshift-storage").List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + worker.NetworkInterfaces[0].IpAddress})
// if err != nil {
// 	return fmt.Errorf("[ERROR] Invalid kubeconfig, failed to list resource: %s", err)

// }

// if err != nil {
// 	panic(err.Error())
// }

// log.Println("This is the Worker Name", worker.NetworkInterfaces[0].IpAddress)

// log.Println("\nIn preWorkerReplace for ODF")
// for _, pod := range pods.Items {
// 	fmt.Printf("Pod name: %v\n", pod.Name)
// 	deploymentName := pod.Labels["app"] + "-" + pod.Labels["ceph-osd-id"]
// 	s, err := clientset.AppsV1().Deployments("openshift-storage").GetScale(context.TODO(), deploymentName, metav1.GetOptions{})
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	sc := *s
// 	sc.Spec.Replicas = 0
// 	scaleDeployment, err := clientset.AppsV1().Deployments("openshift-storage").UpdateScale(context.TODO(), deploymentName, &sc, metav1.UpdateOptions{})
// 	if err != nil {
// 		fmt.Printf("error updating scale object: %v\n", err)
// 		os.Exit(1)
// 	}
// 	fmt.Printf("Successfully scaled deployment %s to %d replicas", deploymentName, scaleDeployment.Spec.Replicas)
// }

// // Crash Collector

// deploymentName := "rook-ceph-crashcollector-10.248.0.99"
// s, err := clientset.AppsV1().Deployments("openshift-storage").GetScale(context.TODO(), deploymentName, metav1.GetOptions{})
// if err != nil {
// 	log.Fatal(err)
// }
// sc := *s
// sc.Spec.Replicas = 0
// scaleDeployment, err := clientset.AppsV1().Deployments("openshift-storage").UpdateScale(context.TODO(), deploymentName, &sc, metav1.UpdateOptions{})
// if err != nil {
// 	fmt.Printf("error updating scale object: %v\n", err)
// 	os.Exit(1)
// }
// fmt.Printf("Successfully scaled deployment %s to %d replicas", deploymentName, scaleDeployment.Spec.Replicas)
