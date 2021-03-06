package k8s

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cpg1111/maestrod/config"
	"github.com/cpg1111/maestrod/manager"
)

// Driver is a struct for the k8s Driver
type Driver struct {
	manager.Driver
	Host           string
	MaestroVersion string
	Client         *http.Client
	Mounts         []config.Mount
}

// New returns a pointer to a k8s Driver
func New(maestroVersion string, conf *config.Config) *Driver {
	authTransport, authErr := NewAuthTransport(&conf.Server)
	if authErr != nil {
		panic(authErr)
	}
	return &Driver{
		Host:           manager.GetTarget(&conf.Server),
		MaestroVersion: maestroVersion,
		Client: &http.Client{
			Transport: authTransport,
		},
		Mounts: conf.Mounts,
	}
}

func readRes(res *http.Response, errObj string, status int) error {
	defer res.Body.Close()
	resBody, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return readErr
	}
	if res.StatusCode != status {
		return fmt.Errorf("did not %s, received  %d \n %s", errObj, res.StatusCode, (string)(resBody))
	}
	return nil
}

func (d *Driver) create(url, errObj string, body []byte) error {
	bodyReader := bytes.NewReader(body)
	res, postErr := d.Client.Post(fmt.Sprintf("%s%s", d.Host, url), "application/json", bodyReader)
	if postErr != nil {
		return postErr
	}
	return readRes(res, errObj, 201)
}

func (d *Driver) destroy(url, errObj string, body []byte) error {
	bodyReader := bytes.NewReader(body)
	req, reqErr := http.NewRequest("DELETE", fmt.Sprintf("%s%s", d.Host, url), bodyReader)
	if reqErr != nil {
		return reqErr
	}
	res, delErr := d.Client.Do(req)
	if delErr != nil {
		return delErr
	}
	return readRes(res, errObj, 200)
}

func (d *Driver) check(url string) (bool, error) {
	res, getErr := d.Client.Get(fmt.Sprintf("%s%s", d.Host, url))
	if res.StatusCode == 404 {
		return false, nil
	} else if res.StatusCode == 200 {
		return true, nil
	}
	return false, getErr
}

// CreateNamespace creates a maestro k8s namespace if one does not exist
func (d *Driver) CreateNamespace(namespace string) error {
	exists, checkErr := d.check(fmt.Sprintf("/api/v1/namespaces/%s", namespace))
	if checkErr != nil {
		return checkErr
	} else if exists {
		return nil
	}
	newNamespace := &Namespace{
		Kind:       "Namespace",
		ApiVersion: "v1",
		Metadata: nsMetadata{
			Name:      namespace,
			Namespace: namespace,
		},
	}
	body, marshErr := json.Marshal(newNamespace)
	if marshErr != nil {
		return marshErr
	}
	return d.create("/api/v1/namespaces", "create namespace", body)
}

// CreateSvcAccnt creates a kubernetes svc accnt
func (d *Driver) CreateSvcAccnt(name string) error {
	exists, checkErr := d.check(fmt.Sprintf("/api/v1/namespaces/maestro/serviceaccounts/%s", name))
	if checkErr != nil {
		return checkErr
	} else if exists {
		return nil
	}
	newSvcAccnt := &ServiceAccount{
		Kind:       "ServiceAccount",
		ApiVersion: "v1",
		Metadata: saMetadata{
			Name:      name,
			Namespace: "maestro",
		},
	}
	body, marshErr := json.Marshal(newSvcAccnt)
	if marshErr != nil {
		return marshErr
	}
	return d.create("/api/namepsaces/maestro/serviceaccounts", "create service account", body)
}

func (d *Driver) createPod(newPod *Pod) error {
	body, marshErr := json.Marshal(newPod)
	if marshErr != nil {
		return marshErr
	}
	return d.create("/api/v1/namespaces/maestro/pods", "create maestro worker", body)
}

// DestroyWorker will delete a maestro pod
func (d Driver) DestroyWorker(project, branch string) error {
	kubifiedProj := strings.Replace(project, "/", "-", -1)
	deletePayload := map[string]string{
		"kind":               "Pod",
		"apiVersion":         "v1",
		"gracePeriodSeconds": "5",
	}
	body, marshErr := json.Marshal(deletePayload)
	if marshErr != nil {
		return marshErr
	}
	return d.destroy(
		fmt.Sprintf("/api/v1/namespaces/maestro/pods/%s-%s", kubifiedProj, branch),
		"delete maestro worker",
		body,
	)
}

func (d *Driver) createVolumes(confName, hostVol string) ([]Volume, error) {
	mounts := make([]Volume, len(d.Mounts)+1)
	confVol, volErr := NewVolume(confName, hostVol, d)
	if volErr != nil {
		return mounts, volErr
	}
	mounts[0] = *confVol
	for m := range d.Mounts {
		vol, vErr := NewVolume(d.Mounts[m].Name, d.Mounts[m].Path, d)
		if vErr != nil {
			return mounts, vErr
		}
		mounts[m+1] = *vol
	}
	return mounts, nil
}

func fmtName(name string) string {
	name = strings.Replace(strings.Replace(name, "/", "-", -1), "_", "-", -1)
	if len(name) > 58 {
		return name[0:58]
	}
	return name
}

// Run will run a maestro pod in kubernetes
func (d Driver) Run(name, confTarget, hostVolume string, args []string) error {
	dPtr := &d
	name = fmtName(name)
	vols, vErr := dPtr.createVolumes(fmt.Sprintf("%s-conf", name), hostVolume)
	if vErr != nil {
		return vErr
	}
	mounts := newMounts(vols)
	sec := &secCtx{}
	maestroContainer := NewContainer(d.MaestroVersion, args, mounts, sec)
	newPod := &Pod{
		Kind:       "Pod",
		ApiVersion: "v1",
		Metadata: podMetadata{
			Name:      name,
			Namespace: "maestro",
		},
		Spec: podSpec{
			Volumes:       vols,
			Containers:    []Container{*maestroContainer},
			RestartPolicy: "Never",
		},
	}
	return dPtr.createPod(newPod)
}
