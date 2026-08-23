package store

import (
	"errors"
	"fmt"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
)

// ErrReverseSearchUnsupported is returned by stores that cannot serve RFC 9536
// reverse searches (e.g. relational stores without the needed index). The
// handler maps it to HTTP 501 Not Implemented, as the RFC requires.
var ErrReverseSearchUnsupported = errors.New("reverse search not supported by this storage backend")

// Interface is the storage adapter boundary. It produces canonical registry
// objects (internal/domain) rather than RDAP wire records. The RDAP wire format
// is derived by the query service (internal/service), so storage stays
// decoupled from the protocol representation and a real registry can back it
// with its own model.
type Interface interface {
	LookupDomain(name string) (*domain.Domain, error)
	// GetDomainAggregate returns a domain with its registrar, contacts, and
	// nameservers resolved from a single consistent snapshot. It is the
	// recommended entry point for rendering a domain RDAP response, because it
	// guarantees the response cannot observe a partially-applied update (e.g. a
	// registrar transfer) across the different object reads.
	GetDomainAggregate(name string) (*domain.DomainAggregate, error)
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
	// ReverseSearchDomainsByEntity returns the domains that have an associated
	// entity matching the given RFC 9536 reverse search property+pattern.
	// property is one of "role", "handle", "fn", "email". Stores that cannot
	// serve reverse searches return ErrReverseSearchUnsupported.
	ReverseSearchDomainsByEntity(property, pattern string, limit int) ([]domain.Domain, error)
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
