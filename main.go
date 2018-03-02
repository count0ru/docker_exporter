package main

import (
	"flag"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"strings"
	"time"
)

var containerStateNames = []string{
	"paused",
	"restarting",
	"running",
	"dead",
	"created",
	"exited",
}

type containersState struct {
	id    string
	image string
	name  string
	state string
}

var containersLastState []containersState

var containerStateMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "dockercontainer_state",
	Help: "Docker container status",
},
	[]string{"id", "image", "name", "state"},
)

func getCurrentContainersState() (currentContainersState []containersState, err error) {

	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	containersList, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	for _, container := range containersList {
		currentContainersState = append(currentContainersState, containersState{
			id:    container.ID,
			image: container.Image,
			name:  strings.Replace(container.Names[0], "/", "", -1),
			state: container.State,
		})
	}
	return currentContainersState, err
}

func createContainerMetric(container containersState) {
	for _, nameState := range containerStateNames {
		if nameState == container.state {
			containerStateMetric.With(prometheus.Labels{
				"name":  container.name,
				"state": nameState,
				"id":    container.id,
				"image": container.image,
			}).Set(1)
		} else {
			containerStateMetric.With(prometheus.Labels{
				"name":  container.name,
				"state": nameState,
				"id":    container.id,
				"image": container.image,
			}).Set(0)
		}
	}
}
func deleteContainerMetric(container containersState) {
	for _, nameState := range containerStateNames {
		containerStateMetric.Delete(prometheus.Labels{
			"id":    container.id,
			"state": nameState,
			"image": container.image,
			"name":  container.name,
		})
	}
}

func collectContainerMetrics() {

	containersNewState, err := getCurrentContainersState()
	if err != nil {
		log.Fatalf("cat get docker containers metrics ", err)
	}

	var isDeleted bool

	for _, lastContainer := range containersLastState {
		isDeleted = true
		for _, newContainer := range containersNewState {
			if lastContainer.id == newContainer.id {
				isDeleted = false
				createContainerMetric(newContainer)
			}
		}
		if isDeleted {
			fmt.Println(lastContainer.name, " with ID ", lastContainer.id, " deleted")
			deleteContainerMetric(lastContainer)
		}
	}
	containersLastState = containersNewState
}

func main() {

	var err error

	listenAddressPtr := flag.String("web.listen-address", ":9188", "Address on which to expose metrics and web interface.")
	flag.Parse()

	listenAddress := *listenAddressPtr

	prometheus.MustRegister(containerStateMetric)

	containersLastState, err = getCurrentContainersState()
	if err != nil {
		log.Fatalf("cat get docker containers metrics ", err)
	}

	collectContainerMetrics()

	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for _ = range ticker.C {
			collectContainerMetrics()
		}
	}()
	log.Print("Listen on http://", listenAddress, "/metric")
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
