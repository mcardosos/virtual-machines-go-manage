---
services: virtual-machines
platforms: go
author: mcardosos
---

# Azure Virtual Machine Management Sample using Azure SDK for Go

This sample demonstrates how to manage your Azure virtual machine (VM) using Go, and specifically how to:

- Create a virtual machine
- Tag a virtual machine
- Attach and detach data disks
- Update the OS disk size
- Start, restart and stop a virtual machine
- List virtual machines
- Delete a virtual machine

If you don't have a Microsoft Azure subscription you can get a FREE trial account [here](https://azure.microsoft.com/pricing/free-trial).

**On this page**

- [Run this sample](#run)
- [What does example.go do?](#sample)
- [More information](#info)

<a id="run"></a>

## Run this sample

1. If you don't already have it, [install Go 1.8](https://golang.org/dl/).

1. Clone the repository.

    ```
    git clone https://github.com:Azure-Samples/virtual-machines-go-manage.git
    ```

1. Install the dependencies using glide.

    ```
    cd virtual-machines-go-manage
    glide install
    ```

1. Create an Azure service principal and authentication file following [this instructions](https://docs.microsoft.com/en-us/python/azure/python-sdk-azure-authenticate?view=azure-python#mgmt-auth-file)

1. Set the `AZURE_AUTH_LOCATION` environment variable with the authentication file path you created on the last step.

    ```
    export AZURE_AUTH_LOCATION={your auth file path}
    ```

    > [AZURE.NOTE] On Windows, use `set` instead of `export`.

1. Run the sample.

    ```
    go run example.go
    ```

<a id="sample"></a>

## What does example.go do?

First, it creates all resources needed before creating a VM (resource group, storage account, virtual network, subnet)

### Creates VMs

This sample creates both a Linux and a Windows VM.

```go
publicIPaddress, nicParameters := createPIPandNIC(vmName, subnet)
vm := setVMparameters(vmName, publisher, offer, sku, *nicParameters.ID)
vmClient.CreateOrUpdate(groupName, vmName, vm, nil)
```

### Get the VM properties

```go
vm, err := vmClient.Get(groupName, vmName, compute.InstanceView)
```

### Tag the VM

```go
vm.Tags = &(map[string]*string{
    "who rocks": to.StringPtr("golang"),
    "where":     to.StringPtr("on azure"),
})
vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
```

### Attach data disks to the VM

```go
vm.StorageProfile.DataDisks = &[]compute.DataDisk{
    {
        Lun:  to.Int32Ptr(0),
        Name: to.StringPtr("dataDisk"),
        Vhd: &compute.VirtualHardDisk{
            URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, fmt.Sprintf("dataDisks-%v", vmName))),
        },
        CreateOption: compute.Empty,
        DiskSizeGB:   to.Int32Ptr(1),
    },
}
vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
```

### Detach data disks

```go 
vm.StorageProfile.DataDisks = &[]compute.DataDisk{}
vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
```

### Updates the VM's OS disk size

```go
if vm.StorageProfile.OsDisk.DiskSizeGB == nil {
    vm.StorageProfile.OsDisk.DiskSizeGB = to.Int32Ptr(0)
}

vmClient.Deallocate(groupName, vmName, nil)

if *vm.StorageProfile.OsDisk.DiskSizeGB <= 0 {
    *vm.StorageProfile.OsDisk.DiskSizeGB = 256
}
*vm.StorageProfile.OsDisk.DiskSizeGB += 10

mClient.CreateOrUpdate(groupName, vmName, *vm, nil)
```

### Starts, restarts and stops the VM

```go
vmClient.Start(groupName, vmName, nil)
vmClient.Restart(groupName, vmName, nil)
vmClient.PowerOff(groupName, vmName, nil)

```

### Lists all VMs in your subscription.

```go
vmList, err := vmClient.ListAll()
```

### Deletes VMs and other resources

```go
vmClient.Delete(groupName, vmName, nil)
groupClient.Delete(groupName, nil)
```

<a id="info"></a>

## More information

- [Windows Virtual Machines documentation](https://azure.microsoft.com/documentation/services/virtual-machines/windows/)
- [Linux Virtual Machines documentation](https://azure.microsoft.com/documentation/services/virtual-machines/linux/)

***

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.