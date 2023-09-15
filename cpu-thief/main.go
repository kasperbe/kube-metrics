package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"io"

	"github.com/google/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProcRequest struct {
	Procs int `json:"max_procs"`
}

type Pool struct {
	Workers []chan bool
}

func kubeCfg(path *string) (*kubernetes.Clientset, error) {
	h := os.Getenv("HOME")
	if h == "" {
		fmt.Println("NO H FOUND!!!")
		return nil, errors.New("couldn't find $HOME directory")
	}

	p := filepath.Join(h, ".kube", "config")
	fmt.Printf("Loaded config at: %s\n", p)
	fmt.Println("Do you wish to continue? y/N")
	r := bufio.NewReader(os.Stdin)

	s, _ := r.ReadString(' ')

	if strings.ToUpper(strings.Trim(s, " ")) != "Y" {
		log.Fatalf("To change the config path, please use --kubecfg\n")
	}

	config, err := clientcmd.BuildConfigFromFlags("", p)
	if err != nil {
		return nil, fmt.Errorf("config flags: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("new config: %v", err)
	}

	return clientset, nil
}

func deployment(replicas int32) *appsv1.Deployment {
	name := fmt.Sprintf("cpu-thief-%s", uuid.NewString())
	qt, err := resource.ParseQuantity("50m")
	if err != nil {
		log.Fatalf("Wrong quantity %v\n", err)
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "demo",
				},
			},
			Replicas: &replicas,
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "demo",
					},
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "cpu-thief",
							Image: "kasperbe/cpu-thief:latest",
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 8080,
								},
							},
							Resources: apiv1.ResourceRequirements{
								Requests: apiv1.ResourceList{
									"cpu": qt,
								},
							},
						},
					},
				},
			},
		},
	}
}

func main() {

	deploy := flag.Bool("deploy", false, "Whether this app should be deployed to kubernetes.")
	cfgPath := flag.String("kubecfg", "", "Absolute path to a working kube config.")
	replicas := flag.Int("replicas", 1, "How many instances to deploy")
	flag.Parse()

	if *deploy == true {
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			log.Fatalf("client with opts: %v", err)
		}

		cli.ImageBuild(context.Background(), strings.NewReader(""), types.ImageBuildOptions{})
		return

		clientset, err := kubeCfg(cfgPath)
		if err != nil {
			log.Fatalf("Can't find a kube cfg at the specified location, trying:\n %s\nPlease specify a path to am ansolute path to a kube cfg by using the --kubecfg flag")
		}

		deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)
		d := deployment(int32(*replicas))

		fmt.Println("Deploying to kubernetes")

		result, err := deploymentsClient.Create(context.Background(), d, metav1.CreateOptions{})
		if err != nil {
			log.Fatalf("deployment: %v", err)
		}

		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt, syscall.SIGTERM)

		<-exit

		deploymentsClient.Delete(context.Background(), result.Name, metav1.DeleteOptions{})

	} else {
		pool := Pool{}
		pool.Workers = []chan bool{}

		fmt.Println("service starting")
		fmt.Println("initial procs available: ", runtime.GOMAXPROCS(0))

		fmt.Println("-------------------------------------")
		fmt.Println("set max procs: ", runtime.GOMAXPROCS(0))

		http.HandleFunc("/procs", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				defer r.Body.Close()
				body, err := io.ReadAll(r.Body)
				if err != nil {
					fmt.Fprintf(w, "invalid body: %v", err)
				}

				var req ProcRequest
				json.Unmarshal(body, &req)

				runtime.GOMAXPROCS(req.Procs)

				for _, c := range pool.Workers {
					c <- true
				}

				pool.Workers = []chan bool{}

				fmt.Printf("Starting %d workers\n", req.Procs)
				for i := 0; i < req.Procs; i++ {
					c := make(chan bool)

					NewWorker(c)

				}
			}
		})

		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatalf("server stopped: %v", err)
		}
	}

}

func NewWorker(c chan bool) {
	go func() {
		tmp := "a"
		for {
			select {
			case <-c:
				fmt.Println(tmp)
				return
			default:
				tmp = "b"

				if tmp == "c" {
					break
				}
			}
		}

	}()
}
