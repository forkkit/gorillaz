package gorillaz

import (
	"flag"
	"github.com/spf13/pflag"
	"log"
)

//Define flags supported by gorillaz
func init() {
	flag.String("env", "dev", "Environment")
	flag.String("conf", "configs", "config folder. default: configs")
	flag.String("log.level", "", "Log level")
	flag.String("service.name", "", "Service name")
	flag.String("service.address", "", "Service address")
	flag.Bool("tracing.enabled", false, "Tracing enabled")
	flag.String("tracing.collector.url", "", "URL of the tracing service")
	flag.Bool("healthcheck.enabled", true, "Healthcheck enabled")
	flag.Bool("pprof.enabled", false, "Pprof enabled")
	flag.Int("pprof.port", 0, "pprof port")
	flag.String("prometheus.endpoint", "/metrics", "Prometheus endpoint")
	flag.Bool("prometheus.enabled", true, "Prometheus enabled")
	flag.Int("http.port", 0, "http port")
	flag.Int("grpc.port", 0, "grpc port")
	flag.Int("metrics.publication.interval.ms", 400, "interval of prometheus metrics publication over gRPC stream")
}

func parseConfiguration(g *Gaz, configPath string) {
	conf := GetConfigPath(configPath)

	const configFilePrefix = "application"
	g.Viper.SetConfigName(configFilePrefix) //the suffix ".properties" will be added by viper
	g.Viper.AddConfigPath(conf)
	g.Viper.SetConfigType("properties")
	err := g.Viper.ReadInConfig()
	if err != nil {
		Sugar.Warnf("unable to read config in path %s with file prefix %s %v", conf, configFilePrefix, err)
	}

	if g.bindConfigKeysAsFlag {
		for _, k := range g.Viper.AllKeys() {
			if flag.Lookup(k) == nil {
				flag.String(k, g.Viper.GetString(k), "flag generated by gorillaz from config file")
			}
		}
	}

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	err = g.Viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("unable to bind flags: %v", err)
	}
}

func GetConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	if f := flag.Lookup("conf"); f != nil {
		return f.Value.String()
	}
	return ""
}
