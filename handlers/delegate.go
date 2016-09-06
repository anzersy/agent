package handlers

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	engineCli "github.com/docker/engine-api/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rancher/agent/core/delegate"
	"github.com/rancher/agent/model"
	"github.com/rancher/agent/utilities/constants"
	"github.com/rancher/agent/utilities/utils"
	revents "github.com/rancher/event-subscriber/events"
	"github.com/rancher/go-rancher/client"
	"golang.org/x/net/context"
)

type DelegateRequestHandler struct {
	dockerClient *engineCli.Client
}

func (h *DelegateRequestHandler) DelegateRequest(event *revents.Event, cli *client.RancherClient) error {
	indata, _ := utils.GetFieldsIfExist(event.Data, "instanceData")
	deEvent, _ := utils.GetFieldsIfExist(event.Data, "event")
	var instanceData model.InstanceData
	if err := mapstructure.Decode(utils.InterfaceToMap(indata), &instanceData); err != nil {
		return errors.Wrap(err, constants.DelegateRequestError)
	}
	var delegateEvent *revents.Event
	if err := mapstructure.Decode(utils.InterfaceToMap(deEvent), &delegateEvent); err != nil {
		return errors.Wrap(err, constants.DelegateRequestError)
	}
	if instanceData.Kind != "container" || instanceData.Token == "" {
		return nil
	}
	instance := model.Instance{
		UUID:       instanceData.UUID,
		AgentID:    instanceData.AgentID,
		ExternalID: instanceData.ExternalID,
	}
	container, err := utils.GetContainer(h.dockerClient, instance, true)
	if err != nil {
		if !utils.IsContainerNotFoundError(err) {
			return errors.Wrap(err, constants.DelegateRequestError)
		}
	}

	if container.ID == "" {
		logrus.Infof("Can not call [%v], container not exist", instance.UUID)
		return errors.New(constants.DelegateRequestError)
	}

	inspect, _ := h.dockerClient.ContainerInspect(context.Background(), container.ID)
	running := inspect.State.Running
	if !running {
		logrus.Error(fmt.Errorf("Can not call [%v], container not running", container.ID))
		return errors.New(constants.DelegateRequestError)
	}
	progress := utils.GetProgress(event, cli)
	exitCode, output, data, err := delegate.NsExec(inspect.State.Pid, delegateEvent)
	if err != nil {
		return errors.Wrap(err, constants.DelegateRequestError)
	}
	if exitCode == 0 {
		return replyWithParent(data, delegateEvent, event, cli)
	}
	progress.Update(fmt.Sprintf("Update fail, exitCode %v, output data %v", exitCode, output))
	return errors.New(constants.DelegateRequestError)
}
