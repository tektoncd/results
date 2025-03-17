package portforward

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForward provides port-forwarding functionality to results api service,
// so cli users would not bother to open a new terminal and type `kubectrl port-forward` manually
type PortForward struct {
	clientConfig *rest.Config
	clientSet    kubernetes.Interface
}

// NewPortForward create a new PortForward to do port-forwarding staff
func NewPortForward() (*PortForward, error) {
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), nil)
	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}

	return &PortForward{clientSet: clientSet, clientConfig: clientConfig}, nil
}

// ForwardPortBackground do port-forwarding in background.
// stopChan control when port-forwarding stops, port specify which port on localhost port-forwarding will occupy
func (pf *PortForward) ForwardPortBackground(stopChan <-chan struct{}, port int) error {
	resultsAPIService, err := pf.clientSet.CoreV1().Services("tekton-pipelines").Get(context.TODO(), "tekton-results-api-service", metav1.GetOptions{})
	if err != nil {
		return err
	}
	resultsPodSelector := labels.Set(resultsAPIService.Spec.Selector)
	pods, err := pf.clientSet.CoreV1().Pods("tekton-pipelines").List(context.TODO(), metav1.ListOptions{
		LabelSelector: resultsPodSelector.String(),
	})
	if err != nil {
		return err
	}
	resultsAPIPod := pods.Items[0]
	req := pf.clientSet.CoreV1().RESTClient().Post().Namespace("tekton-pipelines").Resource("pods").Name(resultsAPIPod.Name).SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(pf.clientConfig)
	if err != nil {
		return err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	readyChan := make(chan struct{})
	fw, err := portforward.NewOnAddresses(dialer, []string{"localhost"}, []string{fmt.Sprintf("%d:8080", port)}, stopChan, readyChan, nil, os.Stderr)
	if err != nil {
		return err
	}
	go func() {
		err := fw.ForwardPorts()
		if err != nil {
			log.Fatalf("err forward ports: %v", err)
		}
	}()

	// wait for port-forward ready
	<-readyChan
	return nil
}

// PickFreePort asks the kernel for a free open port that is ready to use.
func PickFreePort() (port int, err error) {
	var a *net.TCPAddr
	a, err = net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	var l *net.TCPListener
	l, err = net.ListenTCP("tcp", a)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
