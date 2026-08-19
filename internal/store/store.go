package store

import (
	"fmt"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
)

// Interface is the storage adapter boundary. It produces canonical registry
// objects (internal/domain) rather than RDAP wire records. The RDAP wire format
// is derived by the query service (internal/service), so storage stays
// decoupled from the protocol representation and a real registry can back it
// with its own model.
type Interface interface {
	LookupDomain(name string) (*domain.Domain, error)
	LookupContact(handle string) (*domain.Contact, error)
	LookupNameserver(name string) (*domain.NameServer, error)
	LookupIPNetwork(cidr string) (*domain.IPNetwork, error)
	LookupAutnum(asn int) (*domain.Autnum, error)
	SearchDomainsByName(pattern string, limit int) ([]domain.Domain, error)
	SearchDomainsByNS(nsName string, limit int) ([]domain.Domain, error)
	SearchContactsByName(pattern string, limit int) ([]domain.Contact, error)
	SearchContactsByHandle(pattern string, limit int) ([]domain.Contact, error)
	SearchNameserversByName(pattern string, limit int) ([]domain.NameServer, error)
	SearchNameserversByIP(ip string, limit int) ([]domain.NameServer, error)
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
