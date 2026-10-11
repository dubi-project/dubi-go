# empty

`empty` is a small custom module example for `dubi-go`.

```go
package main

import (
	"github.com/dubi-project/dubi-go"
	"github.com/dubi-project/dubi-go/empty"
)

func init() {
	// Importing empty performs this automatically.
}
```

The package singleton `empty.Module` manages two components: `Filter` and
`Handler`. Importing the package mounts the module and records its
`dubi.Host`; applications register the components they need. The module participates in the normal
`Setup → Open → Start → Stop → Close` lifecycle. Application modules do not
need to be placed in `main`; they can register themselves from their package.
