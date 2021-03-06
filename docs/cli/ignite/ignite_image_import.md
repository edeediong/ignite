## ignite image import

Import a new base image for VMs

### Synopsis


Import an OCI image as a base image for VMs, takes in a Docker image identifier.
This importing is done automatically when the "run" or "create" commands are run.
The import step is essentially a cache for images to be used later when running VMs.


```
ignite image import <OCI image> [flags]
```

### Options

```
  -h, --help                         help for import
      --registry-config-dir string   Directory containing the registry configuration (default ~/.docker/)
      --runtime runtime              Container runtime to use. Available options are: [docker containerd] (default containerd)
```

### Options inherited from parent commands

```
      --ignite-config string   Ignite configuration path; refer to the 'Ignite Configuration' docs for more details
      --log-level loglevel     Specify the loglevel for the program (default info)
  -q, --quiet                  The quiet mode allows for machine-parsable output by printing only IDs
```

### SEE ALSO

* [ignite image](ignite_image.md)	 - Manage base images for VMs

