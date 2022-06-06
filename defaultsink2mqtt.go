package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/lawl/pulseaudio"
	"golang.org/x/net/trace"
)

var (
	listenAddress = flag.String("listen",
		":8718",
		"listen address for HTTP API (e.g. for Shelly buttons)")

	mqttBroker = flag.String("mqtt_broker",
		"tcp://dr.lan:1883",
		"MQTT broker address for github.com/eclipse/paho.mqtt.golang")

	mqttPrefix = flag.String("mqtt_topic",
		"github.com/stapelberg/defaultsink2mqtt/",
		"MQTT topic prefix")
)

func defaultsink2mqtt() error {
	opts := mqtt.NewClientOptions().AddBroker(*mqttBroker)
	clientID := "https://github.com/stapelberg/defaultsink2mqtt"
	if hostname, err := os.Hostname(); err == nil {
		clientID += "@" + hostname
	}
	opts.SetClientID(clientID)
	opts.SetConnectRetry(true)
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("MQTT connection failed: %v", token.Error())
	}

	trace.AuthRequest = func(req *http.Request) (any, sensitive bool) { return true, true }

	client, err := pulseaudio.NewClient()
	if err != nil {
		return err
	}
	defer client.Close()
	updates, err := client.Updates()
	if err != nil {
		return err
	}
	go func() {
		var lastDefaultSink string
		for ; ; <-updates {
			info, err := client.ServerInfo()
			if err != nil {
				log.Printf("ServerInfo: %v", err)
				continue
			}
			if info.DefaultSink != lastDefaultSink {
				log.Printf("default sink changed from %s to %s", lastDefaultSink, info.DefaultSink)

				mqttClient.Publish(
					*mqttPrefix+"default_sink",
					0,    /* qos */
					true, /* retained */
					string(info.DefaultSink))

				lastDefaultSink = info.DefaultSink
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/requests/", trace.Traces)

	log.Printf("http.ListenAndServe(%q)", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, mux); err != nil {
		return err
	}
	return nil
}

func main() {
	flag.Parse()
	if err := defaultsink2mqtt(); err != nil {
		log.Fatal(err)
	}
}
