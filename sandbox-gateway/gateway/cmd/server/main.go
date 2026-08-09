package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"sandbox-gateway/internal/api"
	"sandbox-gateway/internal/config"
	"sandbox-gateway/internal/database"
	"sandbox-gateway/internal/driver"
	dockerdriver "sandbox-gateway/internal/driver/docker"
	k8sdriver "sandbox-gateway/internal/driver/kubernetes"
	"sandbox-gateway/internal/logging"
	"sandbox-gateway/internal/service"
	"sandbox-gateway/internal/store"

	"github.com/rs/zerolog/log"
)

func main() {
	logging.Init("sandbox-gateway")

	configPath := flag.String("config", envOr("SBGW_CONFIG", "config.yaml"), "path to config YAML")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal().Err(err).Str("config_path", *configPath).Msg("load config failed")
	}

	drv, err := buildDriver(cfg)
	if err != nil {
		log.Fatal().Err(err).Str("driver", cfg.Driver).Msg("build driver failed")
	}
	log.Info().
		Str("driver", cfg.Driver).
		Str("db_driver", cfg.Database.Driver).
		Str("db_host", cfg.Database.Host).
		Str("db_name", cfg.Database.Name).
		Bool("db_dsn_set", cfg.Database.DSN != "").
		Str("image", cfg.Image.Ref).
		Str("listen", cfg.Server.Listen).
		Msg("gateway starting")

	db, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Str("db_driver", cfg.Database.Driver).Msg("open database failed")
	}
	st := store.New(db)

	svc := service.New(drv, st, service.Config{
		Image:            cfg.Image.Ref,
		ProviderImages:   cfg.Image.ByProvider,
		ImageTemplate:    cfg.Image.Template,
		Ports:            cfg.Image.Ports.Public(),
		InternalPorts:    cfg.Image.Ports.Internal(),
		SessionPort:      cfg.Image.Ports.Session,
		FinalizeTimeout:  cfg.FinalizeTimeout(),
		Resources:        cfg.ResourceDefaults(),
		OrphanGCInterval: cfg.OrphanGCInterval(),
		OrphanGCMinAge:   cfg.OrphanGCMinAge(),
	})

	// Reconcile persisted records against live driver state on startup.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		svc.ReconcileOnStartup(ctx)
	}()

	if cfg.OrphanGC.Enabled {
		go svc.RunOrphanGC(context.Background())
	}

	h := api.NewHandler(svc, cfg.Image.Ports)
	r := api.NewRouter(h, cfg)
	if err := r.Run(cfg.Server.Listen); err != nil {
		log.Fatal().Err(err).Str("listen", cfg.Server.Listen).Msg("http server failed")
	}
}

func buildDriver(cfg *config.Config) (driver.Driver, error) {
	switch cfg.Driver {
	case "docker":
		return dockerdriver.New(dockerdriver.Options{
			BindIP:        cfg.Docker.BindIP,
			Network:       cfg.Docker.Network,
			NamePrefix:    cfg.Docker.NamePrefix,
			ShmSize:       cfg.Docker.ShmSize,
			InternalPorts: cfg.Image.Ports.Internal(),
		}), nil
	case "kubernetes":
		return k8sdriver.New(k8sdriver.Options{
			InCluster:          cfg.K8s.InCluster,
			Kubeconfig:         cfg.K8s.Kubeconfig,
			Namespace:          cfg.K8s.Namespace,
			NamePrefix:         cfg.K8s.NamePrefix,
			StorageClass:       cfg.K8s.StorageClass,
			DataDiskGi:         cfg.K8s.DataDiskGi,
			PVCAnnotations:     cfg.K8s.PVCAnnotations,
			ImagePullSecret:    cfg.K8s.ImagePullSecret,
			ImagePullPolicy:    cfg.K8s.ImagePullPolicy,
			EnableLoadBalancer: cfg.K8s.EnableLoadBalancer,
			CPUCores:           cfg.K8s.CPUCores,
			MemoryMB:           cfg.K8s.MemoryMB,
			CPURequestCores:    cfg.K8s.CPURequestCores,
			MemoryRequestMB:    cfg.K8s.MemoryRequestMB,
			CPURequestRatio:    cfg.K8s.CPURequestRatio,
			MemoryRequestRatio: cfg.K8s.MemoryRequestRatio,
			PublicPorts:        cfg.Image.Ports.Public(),
			InternalPorts:      cfg.Image.Ports.Internal(),
		})
	default:
		return nil, fmt.Errorf("unknown driver %q", cfg.Driver)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
