/*
Copyright 2019 The Knative Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generators

import (
	"fmt"
	"io"

	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"
)

// clientGenerator produces a file of listers for a given GroupVersion and
// type.
type wiresetGenerator struct {
	generator.DefaultGen
	outputPackage                string
	imports                      namer.ImportTracker
	clientSetPackage             string
	sharedInformerFactoryPackage string
}

var _ generator.Generator = (*clientGenerator)(nil)

func (g *wiresetGenerator) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *wiresetGenerator) Imports(c *generator.Context) []string {
	return g.imports.ImportLines()
}

func (g *wiresetGenerator) PackageVars(c *generator.Context) []string {
	namer := c.Namers["raw"]
	wireNewSet := c.Universe.Package("github.com/google/wire").Function("NewSet")
	wireBind := c.Universe.Package("github.com/google/wire").Function("Bind")
	newForConfig := c.Universe.Package(g.clientSetPackage).Function("NewForConfig")
	clientSet := c.Universe.Type(types.Name{Package: g.clientSetPackage, Name: "Clientset"})
	clientSetInterface := c.Universe.Type(types.Name{Package: g.clientSetPackage, Name: "Interface"})
	g.Namers(c)
	return []string{
		fmt.Sprintf("ClientSet = %s(", namer.Name(wireNewSet)),
		fmt.Sprintf("%s,", namer.Name(newForConfig)),
		fmt.Sprintf("%s(new(%s), new(*%s)),", namer.Name(wireBind), namer.Name(clientSetInterface), namer.Name(clientSet)),
		"NewInformerFactory,",
		")",
	}
}

func (g *wiresetGenerator) Init(c *generator.Context, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "{{", "}}")

	m := map[string]interface{}{
		"informerFactories": c.Universe.Type(types.Name{Package: "knative.dev/pkg/injection/wire", Name: "InformerFactories"}),
		"clientSet":         c.Universe.Type(types.Name{Package: g.clientSetPackage, Name: "Clientset"}),
		"informersNewSharedInformerFactoryWithOptions": c.Universe.Function(types.Name{Package: g.sharedInformerFactoryPackage, Name: "NewSharedInformerFactoryWithOptions"}),
		"informersSharedInformerOption":                c.Universe.Function(types.Name{Package: g.sharedInformerFactoryPackage, Name: "SharedInformerOption"}),
		"informersWithNamespace":                       c.Universe.Function(types.Name{Package: g.sharedInformerFactoryPackage, Name: "WithNamespace"}),
		"informersSharedInformerFactory":               c.Universe.Function(types.Name{Package: g.sharedInformerFactoryPackage, Name: "SharedInformerFactory"}),
		"injectionHasNamespace":                        c.Universe.Type(types.Name{Package: "knative.dev/pkg/injection", Name: "HasNamespaceScope"}),
		"injectionGetNamespace":                        c.Universe.Type(types.Name{Package: "knative.dev/pkg/injection", Name: "GetNamespaceScope"}),
		"controllerGetResyncPeriod":                    c.Universe.Type(types.Name{Package: "knative.dev/pkg/controller", Name: "GetResyncPeriod"}),
		"contextContext": c.Universe.Type(types.Name{
			Package: "context",
			Name:    "Context",
		}),
	}
	sw.Do(provideFactoryTemplate, m)

	return sw.Error()
}

const provideFactoryTemplate = `
func NewInformerFactory(ctx {{.contextContext|raw}}, c *{{.clientSet|raw}}, fs *{{.informerFactories|raw}}) {{.informersSharedInformerFactory|raw}} {
	opts := make([]{{.informersSharedInformerOption|raw}}, 0, 1)
	if {{.injectionHasNamespace|raw}}(ctx) {
		opts = append(opts, {{.informersWithNamespace|raw}}({{.injectionGetNamespace|raw}}(ctx)))
	}
	f := {{.informersNewSharedInformerFactoryWithOptions|raw}}(c, {{.controllerGetResyncPeriod|raw}}(ctx), opts...)
        fs.AddFactory(f)
        return f
}
`
