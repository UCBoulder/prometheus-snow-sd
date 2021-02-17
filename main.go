// Copyright 2018 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/documentation/examples/custom-sd/adapter"
)

var (
	a             = kingpin.New("sd adapter usage", "Tool to generate file_sd target files for unimplemented SD mechanisms.")
	outputFile    = a.Flag("output.file", "Output file for file_sd compatible file.").Default("custom_sd.json").String()
	snowInstance = a.Flag("snow.instance", "The ServiceNow instance: X.service-now.com.").Default("example").String()
	cmdbTable = a.Flag("cmdb.table", "The ServiceNow table that contains your desired CI objects.").Default("cmdb_ci_linux_server").String()
	sysparmQuery = a.Flag("sysparm.query", "An optional encoded query string used to filter the results.").Default("").String()
	logger        log.Logger

	// addressLabel is the name for the label containing a target's address.
	addressLabel = model.MetaLabelPrefix + "sn_fqdn"
	// nodeLabel is the name for the label containing a target's node name.
	nodeLabel = model.MetaLabelPrefix + "sn_name"
	// classLabel is the name of the label containing the classes assigned to the target.
	classLabel = model.MetaLabelPrefix + "sn_class"
	// metaLabel is the name of the label containing the metadata assigned to the target.
	metaLabel = model.MetaLabelPrefix + "sn_meta"
)

// CatalogService is reconstructed from SNOW REST explorer
// this struct represents the response from a /api/now/table/cmdb_ci_linux_server request
type CatalogService struct {
	fqdn			   string
	u_systemlist_class string
}

// Note: create a config struct for your custom SD type here.
type sdConfig struct {
	Address         string
	Table           string
	SysparmQuery    string
	RefreshInterval int
}

// Note: This is the struct with your implementation of the Discoverer interface (see Run function).
// Discovery retrieves target information from a Consul server and updates them via watches.
type discovery struct {
	address         string
	refreshInterval int
	cmdbTable       string
	sysparmQuery    string
	logger          log.Logger
}

func (d *discovery) parseServiceNodes(resp *http.Response, name string) (*targetgroup.Group, error) {
	return nil, nil
}

// Note: you must implement this function for your discovery implementation as part of the
// Discoverer interface. Here you should query your SD for it's list of known targets, determine
// which of those targets you care about (for example, which of Consuls known services do you want
// to scrape for metrics), and then send those targets as a target.TargetGroup to the ch channel.
func (d *discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	for c := time.Tick(time.Duration(d.refreshInterval) * time.Second); ; {
		var nodes map[string][]map[string]string

		url := fmt.Sprintf(
			"https://%s.service-now.com/api/now/table/%s?sysparm_query=%s",
			d.address,
			d.cmdbTable,
			d.sysparmQuery,
		)
		level.Debug(d.logger).Log("url", url)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			level.Error(d.logger).Log("msg", "Error getting services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			b, _ := json.MarshalIndent(req, "", " ")
			level.Debug(d.logger).Log("req", string(b))
			continue
		}
	
		req.SetBasicAuth(os.Getenv("SNOW_USER"), os.Getenv("SNOW_PASS"))
		req.Header.Add("Accept", "application/json")
		req.Close = true

		client := http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			level.Error(d.logger).Log("msg", "Error getting services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&nodes)
		resp.Body.Close()
		if err != nil {
			level.Error(d.logger).Log("msg", "Error reading services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

        b, err := json.MarshalIndent(nodes, "", " ")
		level.Debug(d.logger).Log("msg", string(b), "err", nil)

		tgroup := targetgroup.Group{
			Source: "snow",
			Labels: make(model.LabelSet),
		}

		tgroup.Targets = make([]model.LabelSet, 0, len(nodes))

		for _, node := range nodes["result"] {
			if node["fqdn"] == "" {
				continue
			}
			target := model.LabelSet{
				model.AddressLabel:          model.LabelValue(node["fqdn"]),
				model.LabelName(classLabel): model.LabelValue(node["u_systemlist_class"]),
			}
			tgroup.Targets = append(tgroup.Targets, target)
			level.Debug(d.logger).Log("msg", fmt.Sprintf("Found %s", node["fqdn"]), "err", nil)
		}

		var tgs []*targetgroup.Group
		tgs = append(tgs, &tgroup)

		if err == nil {
			ch <- tgs
		}
		// Wait for ticker or exit when ctx is closed.
		select {
		case <-c:
			continue
		case <-ctx.Done():
			return
		}
	}
}

func newDiscovery(conf sdConfig) (*discovery, error) {
	cd := &discovery{
		address:         conf.Address,
		refreshInterval: conf.RefreshInterval,
		cmdbTable:       conf.Table,
		logger:          logger,
		sysparmQuery:    conf.SysparmQuery,
	}
	return cd, nil
}

func main() {
	a.HelpFlag.Short('h')

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Println("err: ", err)
		return
	}
	logger = log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

	ctx := context.Background()

	// NOTE: create an instance of your new SD implementation here.
	cfg := sdConfig{
		Table:           *cmdbTable,
		SysparmQuery:    *sysparmQuery,
		Address:         *snowInstance,
		RefreshInterval: 30,
	}

	disc, err := newDiscovery(cfg)
	if err != nil {
		fmt.Println("err: ", err)
	}
	sdAdapter := adapter.NewAdapter(ctx, *outputFile, "exampleSD", disc, logger)
	sdAdapter.Run()

	<-ctx.Done()
}
