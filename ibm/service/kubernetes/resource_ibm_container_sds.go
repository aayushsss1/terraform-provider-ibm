package kubernetes

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	v2 "github.com/IBM-Cloud/bluemix-go/api/container/containerv2"
	templatev1 "github.com/openshift/api/template/v1"
	templatev1client "github.com/openshift/client-go/template/clientset/versioned/typed/template/v1"
	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/clientcmd"
)

type Sds interface {
	preWorkerReplace(worker v2.Worker, clusterConfig string) error
	postWorkerReplace(worker v2.Worker, clusterConfig string) error
}

var deploymentList []string

type odf struct{}

func (o odf) preWorkerReplace(worker v2.Worker, clusterConfig string) error {

	log.Println("\nInside preWorkerReplace for ODF")

	//1. Load the cluster config
	log.Println("\nThis is the cluster config\n", clusterConfig)

	config, err := clientcmd.BuildConfigFromFlags("", clusterConfig)

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
	err = scalePods(clientset, workerName, "app", "rook-ceph-mon", "mon", 0)
	if err != nil {
		return fmt.Errorf("[ERROR] Error in Scaling")
	}
	// OSD Pods
	err = scalePods(clientset, workerName, "app", "rook-ceph-osd", "ceph-osd-id", 0)
	if err != nil {
		return fmt.Errorf("[ERROR] Error in Scaling")
	}
	// Crash Collector
	err = scalePods(clientset, workerName, "app", "rook-ceph-crashcollector", "node_name", 0)
	if err != nil {
		return fmt.Errorf("[ERROR] Error in Scaling")
	}

	time.Sleep(time.Second * 45)

	node := workerName
	n, err := clientset.CoreV1().Nodes().Get(context.Background(), node, metav1.GetOptions{})

	if err != nil {
		return fmt.Errorf("Node %s not found\n", node)
	}

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

func scalePods(clientset *kubernetes.Clientset, workerName string, appLabel string, appLabelValue string, idLabel string, replicas int32) error {

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
		deploymentList = append(deploymentList, deploymentName)
		result, getErr := deploymentsClient.Get(context.TODO(), deploymentName, metav1.GetOptions{})

		if getErr != nil {
			return fmt.Errorf("Failed to get latest version of deployment: %v", getErr)
		}

		result.Spec.Replicas = int32Ptr(replicas) // reduce replica count
		scaleDeployment, err := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})

		if err != nil {
			return fmt.Errorf("Error updating scale object: %v", err)
		}

		log.Printf("Successfully scaled deployment %s to %d replicas", deploymentName, *scaleDeployment.Spec.Replicas)
	}
	return nil
}

func (o odf) postWorkerReplace(worker v2.Worker, clusterConfig string) error {

	config, err := clientcmd.BuildConfigFromFlags("", clusterConfig)

	if err != nil {
		return fmt.Errorf("[ERROR] Invalid kubeconfig, failed to set context: %s", err)
	}

	//2. create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("[ERROR] Invalid kubeconfig,, failed to create clientset: %s", err)
	}

	deploymentsClient := clientset.AppsV1().Deployments("openshift-storage")

	if err != nil {
		log.Fatal(err)
	}

	for _, v := range deploymentList {

		if !strings.Contains(v, "crashcollector") {

			result, getErr := deploymentsClient.Get(context.TODO(), v, metav1.GetOptions{})

			if getErr != nil {
				return fmt.Errorf("failed to get latest version of deployment: %v", getErr)
			}

			result.Spec.Replicas = int32Ptr(1) // increase replica count
			scaleDeployment, err := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})

			if err != nil {
				return fmt.Errorf("Error updating scale object: %v", err)
			}

			log.Printf("Successfully scaled deployment %s to %d replicas", v, *scaleDeployment.Spec.Replicas)

		}

	}

	pods, err := clientset.CoreV1().Pods("openshift-storage").List(context.TODO(), metav1.ListOptions{})

	if err != nil {

		return fmt.Errorf("Error getting Pods")

	}
	for _, pod := range pods.Items {

		if (pod.Labels["app"] == "rook-ceph-osd") && (pod.Status.Phase != "Running") {

			ExecuteTemplate()

		}
	}

	return nil

}

func int32Ptr(i int32) *int32 { return &i }

func ExecuteTemplate() {

	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	// Determine the Namespace referenced by the current context in the
	// kubeconfig file.
	namespace, _, err := kubeconfig.Namespace()
	if err != nil {
		panic(err)
	}

	// Get a rest.Config from the kubeconfig file.  This will be passed into all
	// the client objects we create.
	restconfig, err := kubeconfig.ClientConfig()
	if err != nil {
		panic(err)
	}

	// Create a Kubernetes core/v1 client.
	coreclient, err := corev1client.NewForConfig(restconfig)
	if err != nil {
		panic(err)
	}

	// Create an OpenShift template/v1 client.
	templateclient, err := templatev1client.NewForConfig(restconfig)
	if err != nil {
		panic(err)
	}

	// Get the "ocs-osd-removal" Template from the "openshift" Namespace.
	template, err := templateclient.Templates("openshift-storage").Get(context.Background(),
		"ocs-osd-removal", metav1.GetOptions{})
	if err != nil {
		panic(err)
	}
	// INSTANTIATE THE TEMPLATE.

	// To set Template parameters, create a Secret holding overridden parameters
	// and their values.
	secret, err := coreclient.Secrets(namespace).Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "parameters",
		},
		StringData: map[string]string{
			"FAILED_OSD_IDS": "1",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	// Create a TemplateInstance object, linking the Template and a reference to
	// the Secret object created above.
	ti, err := templateclient.TemplateInstances(namespace).Create(context.Background(),
		&templatev1.TemplateInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name: "templateinstance",
			},
			Spec: templatev1.TemplateInstanceSpec{
				Template: *template,
				Secret: &corev1.LocalObjectReference{
					Name: secret.Name,
				},
			},
		}, metav1.CreateOptions{})

	if err != nil {
		panic(err)
	}

	// Watch the TemplateInstance object until it indicates the Ready or
	// InstantiateFailure status condition.
	watcher, err := templateclient.TemplateInstances(namespace).Watch(context.Background(),
		metav1.SingleObject(ti.ObjectMeta),
	)
	if err != nil {
		panic(err)
	}

	for event := range watcher.ResultChan() {
		switch event.Type {
		case watch.Modified:
			ti = event.Object.(*templatev1.TemplateInstance)

			for _, cond := range ti.Status.Conditions {
				// If the TemplateInstance contains a status condition
				// Ready == True, stop watching.
				if cond.Type == templatev1.TemplateInstanceReady &&
					cond.Status == corev1.ConditionTrue {
					watcher.Stop()
				}

				// If the TemplateInstance contains a status condition
				// InstantiateFailure == True, indicate failure.
				if cond.Type ==
					templatev1.TemplateInstanceInstantiateFailure &&
					cond.Status == corev1.ConditionTrue {
					panic("templateinstance instantiation failed")
				}
			}

		default:
			panic("unexpected event type " + event.Type)
		}
	}

	// DELETE THE INSTANTIATED TEMPLATE.

	// We use the foreground propagation policy to ensure that the garbage
	// collector removes all instantiated objects before the TemplateInstance
	// itself disappears.
	foreground := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{PropagationPolicy: &foreground}
	err = templateclient.TemplateInstances(namespace).Delete(context.Background(), ti.Name,
		deleteOptions)
	if err != nil {
		panic(err)
	}

	// Watch the TemplateInstance object until it disappears.
	watcher, err = templateclient.TemplateInstances(namespace).Watch(context.Background(),
		metav1.SingleObject(ti.ObjectMeta),
	)
	if err != nil {
		panic(err)
	}

	for event := range watcher.ResultChan() {
		switch event.Type {
		case watch.Modified:
			// do nothing

		case watch.Deleted:
			watcher.Stop()

		default:
			panic("unexpected event type " + event.Type)
		}
	}

	// Finally delete the "parameters" Secret.
	err = coreclient.Secrets(namespace).Delete(context.Background(), secret.Name,
		metav1.DeleteOptions{})
	if err != nil {
		panic(err)
	}
}
