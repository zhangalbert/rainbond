// RAINBOND, Application Management Platform
// Copyright (C) 2014-2017 Goodrain Co., Ltd.

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version. For any non-GPL usage of Rainbond,
// one or multiple Commercial Licenses authorized by Goodrain Co., Ltd.
// must be obtained first.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/urfave/cli"
	//"github.com/goodrain/rainbond/pkg/grctl/clients"
	"fmt"

	"github.com/goodrain/rainbond/pkg/grctl/clients"
	"github.com/goodrain/rainbond/pkg/node/api/model"
	"github.com/goodrain/rainbond/pkg/util"
)

func NewCmdInit() cli.Command {
	c := cli.Command{
		Name: "init",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "etcd",
				Usage: "etcd ip,127.0.0.1",
			},
			cli.StringFlag{
				Name:  "type",
				Usage: "node type:manage/compute, manage",
			},
			cli.StringFlag{
				Name:  "mip",
				Usage: "当前节点内网IP, 10.0.0.1",
			},
			cli.StringFlag{
				Name:  "repo_ver",
				Usage: "repo version,3.4",
			},
			cli.StringFlag{
				Name:  "install_type",
				Usage: "online/offline ,online",
			},
		},
		Usage: "初始化集群。grctl init cluster",
		Action: func(c *cli.Context) error {
			return initCluster(c)
		},
	}
	return c
}
func NewCmdInstallStatus() cli.Command {
	c := cli.Command{
		Name: "install_status",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "taskID",
				Usage: "install_k8s,空则自动寻找",
			},
		},
		Usage: "获取task执行状态。grctl install_status",
		Action: func(c *cli.Context) error {
			taskID := c.String("taskID")
			if taskID == "" {
				tasks, err := clients.NodeClient.Tasks().List()
				if err != nil {
					logrus.Errorf("error get task list,details %s", err.Error())
					return nil
				}
				for _, v := range tasks {
					for _, vs := range v.Status {
						if vs.Status == "start" || vs.Status == "create" {
							//Status(v.ID)
							return nil
						}

					}
				}
			} else {
				//Status(taskID)
			}
			return nil
		},
	}
	return c
}

func initCluster(c *cli.Context) error {
	resp, err := http.Get("http://repo.goodrain.com/gaops/jobs/install/prepare/init.sh")

	if err != nil {
		logrus.Errorf("error get init script,details %s", err.Error())
		return err
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)
	args := []string{c.String("etcd"), c.String("type"), c.String("mip"), c.String("repo_ver"), c.String("install_type")}
	arg := strings.Join(args, " ")
	argCheck := strings.Join(args, "")
	if len(argCheck) > 0 {
		arg += ";"
	} else {
		arg = ""
	}
	fmt.Println("begin init cluster first node,please don't exit,wait install")
	cmd := exec.Command("bash", "-c", arg+string(b))
	var result []byte
	cmd.Stderr = bytes.NewBuffer(result)
	stdout, _ := cmd.StdoutPipe()
	go func() {
		read := bufio.NewReader(stdout)
		for {
			line, _, err := read.ReadLine()
			if err != nil {
				return
			}
			fmt.Println(line)
		}
	}()
	if err := cmd.Run(); err != nil {
		logrus.Errorf("current node init error,%s", err.Error())
		return err
	}
	//检测并设置init的结果
	index := strings.Index(string(result), "{")
	jsonOutPut := string(result)
	if index > -1 {
		jsonOutPut = string(result)[index:]
	}
	output, err := model.ParseTaskOutPut(jsonOutPut)
	if err != nil {
		logrus.Errorf("get init current node result error:%s", err.Error())
		return err
	}
	var newConfigs []model.ConfigUnit
	if output.Global != nil {
		for k, v := range output.Global {
			if strings.Index(v, "|") > -1 {
				values := strings.Split(v, "|")
				newConfigs = append(newConfigs, model.ConfigUnit{
					Name:           strings.ToUpper(k),
					Value:          values,
					ValueType:      "array",
					IsConfigurable: false,
				})
			} else {
				newConfigs = append(newConfigs, model.ConfigUnit{
					Name:           strings.ToUpper(k),
					Value:          v,
					ValueType:      "string",
					IsConfigurable: false,
				})
			}
		}
	}
	var gc *model.GlobalConfig
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second * 2)
		gc, err = clients.NodeClient.Configs().Get()
		if err == nil && gc != nil {
			for _, nc := range newConfigs {
				gc.Add(nc)
			}
			err = clients.NodeClient.Configs().Put(gc)
			break
		}
	}
	if err != nil {
		logrus.Errorf("Update Datacenter configs error,please check node status")
		return err
	}
	//获取当前节点ID
	hostID, err := util.ReadHostID("")
	if err != nil {
		logrus.Errorf("read nodeid error,please check node status")
		return err
	}

	err = clients.NodeClient.Tasks().Exec("check_manage_base_services", []string{hostID})
	if err != nil {
		logrus.Errorf("error exec task:%s,details %s", "check_manage_base_services", err.Error())
		return err
	}
	err = clients.NodeClient.Tasks().Exec("check_manage_services", []string{hostID})
	if err != nil {
		logrus.Errorf("error exec task:%s,details %s", "check_manage_services", err.Error())
		return err
	}
	Status("check_manage_base_services", []string{hostID})
	Status("check_manage_services", []string{hostID})

	fmt.Println("install manage node success,next you can :")
	fmt.Println("	add compute node--grctl node add -h")
	fmt.Println("	install compute node--grctl install compute -h")
	fmt.Println("	up compute node--grctl node up -h")
	return nil
}
