package store

import (
	"fmt"

	"github.com/rdap-server/rdap/internal/config"
	"github.com/rdap-server/rdap/internal/rdap"
)

type Interface interface {
	LookupDomain(name string) (*rdap.DomainRecord, error)
	LookupEntity(handle string) (*rdap.EntityRecord, error)
	LookupNameserver(name string) (*rdap.NameserverRecord, error)
	LookupIPNetwork(cidr string) (*rdap.IPNetworkRecord, error)
	SearchDomainsByName(pattern string, limit int) ([]rdap.DomainRecord, error)
	SearchDomainsByNS(nsName string, limit int) ([]rdap.DomainRecord, error)
	SearchEntitiesByName(pattern string, limit int) ([]rdap.EntityRecord, error)
	SearchEntitiesByHandle(pattern string, limit int) ([]rdap.EntityRecord, error)
	SearchNameserversByName(pattern string, limit int) ([]rdap.NameserverRecord, error)
	SearchNameserversByIP(ip string, limit int) ([]rdap.NameserverRecord, error)
	Ping() error
	Close() error
}

func New(cfg config.StorageConfig) (Interface, error) {
	switch cfg.Driver {
	case "memory":
		return NewMemoryStore(cfg)
	case "postgres":
		return NewPostgresStore(cfg)
	case "mysql":
		return NewMySQLStore(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage driver: %s", cfg.Driver)
	}
}
