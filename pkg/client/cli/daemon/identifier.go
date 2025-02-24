package daemon

import (
	"errors"

	"github.com/telepresenceio/telepresence/v2/pkg/client"
)

type Identifier struct {
	Name        string
	KubeContext string
	Namespace   string
}

func NewIdentifier(name, contextName, namespace string) (*Identifier, error) {
	if namespace == "" {
		return nil, errors.New("daemon identifier must have a namespace")
	}
	if name == "" {
		if contextName == "" {
			// Must be an in-cluster config
			name = "in-cluster-" + namespace
		} else {
			name = contextName + "-" + namespace
		}
	}
	return &Identifier{KubeContext: contextName, Namespace: namespace, Name: SafeContainerName(name)}, nil
}

func (id *Identifier) String() string {
	return id.Name
}

func (id *Identifier) InfoFileName() string {
	return id.String() + ".json"
}

func (id *Identifier) ContainerName() string {
	return "tp-" + id.String()
}

// IdentifierFromFlags returns a unique name created from the name of the current context
// and the active namespace denoted by the given flagMap.
func IdentifierFromFlags(name string, flagMap map[string]string) (*Identifier, error) {
	cld, err := client.ConfigLoader(flagMap)
	if err != nil {
		return nil, err
	}
	ns, _, err := cld.Namespace()
	if err != nil {
		return nil, err
	}

	config, err := cld.RawConfig()
	if err != nil {
		return nil, err
	}
	cc := flagMap["context"]
	if cc == "" {
		cc = config.CurrentContext
	}
	return NewIdentifier(name, cc, ns)
}
