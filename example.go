// This package demonstrates how to manage Azure virtual machines using Go.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
)

const (
	vhdURItemplate = "https://%s.blob.core.windows.net/golangcontainer/%s.vhd"
	linuxVMname    = "linuxVM"
	windowsVMname  = "windowsVM"
)

// This example requires that the following environment vars are set:
//
// AZURE_TENANT_ID: contains your Azure Active Directory tenant ID or domain
// AZURE_CLIENT_ID: contains your Azure Active Directory Application Client ID
// AZURE_CLIENT_SECRET: contains your Azure Active Directory Application Secret
// AZURE_SUBSCRIPTION_ID: contains your Azure Subscription ID
//

var (
	groupName   = "your-azure-sample-group"
	accountName = "golangrocksonazure"
	location    = "westus"

	groupClient      resources.GroupsClient
	accountClient    storage.AccountsClient
	vNetClient       network.VirtualNetworksClient
	subnetClient     network.SubnetsClient
	addressClient    network.PublicIPAddressesClient
	interfacesClient network.InterfacesClient
	vmClient         compute.VirtualMachinesClient
)

func init() {
	subscriptionID := getEnvVarOrExit("AZURE_SUBSCRIPTION_ID")
	tenantID := getEnvVarOrExit("AZURE_TENANT_ID")

	oauthConfig, err := adal.NewOAuthConfig(azure.PublicCloud.ActiveDirectoryEndpoint, tenantID)
	onErrorFail(err, "OAuthConfigForTenant failed")

	clientID := getEnvVarOrExit("AZURE_CLIENT_ID")
	clientSecret := getEnvVarOrExit("AZURE_CLIENT_SECRET")
	spToken, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, azure.PublicCloud.ResourceManagerEndpoint)
	authorizer := autorest.NewBearerAuthorizer(spToken)
	onErrorFail(err, "NewServicePrincipalToken failed")

	createClients(subscriptionID, authorizer)
}

func main() {
	subnet := createNeededResources()
	defer groupClient.Delete(groupName, nil)

	go createVM(linuxVMname, "Canonical", "UbuntuServer", "16.04.0-LTS", subnet)
	createVM(windowsVMname, "MicrosoftWindowsServer", "WindowsServer", "2016-Datacenter", subnet)

	fmt.Println("Your Linux VM and Windows VM have been created")
	fmt.Print("Press enter to perform various operations on the virtual machines...")
	var input string
	fmt.Scanln(&input)

	go vmOperations(linuxVMname)
	vmOperations(windowsVMname)

	listVMs()

	fmt.Print("Press enter to delete the VMs and other resources created in this sample...")
	fmt.Scanln(&input)

	go deleteVM(linuxVMname)
	deleteVM(windowsVMname)

	fmt.Println("Starting to delete the resource group...")
	_, errGroup := groupClient.Delete(groupName, nil)
	onErrorFail(<-errGroup, "Delete resource group failed")
	fmt.Println("... resource group deleted")

	fmt.Println("Done!")
}

// createNeededResources creates all common resources needed before creating VMs.
func createNeededResources() *network.Subnet {
	fmt.Println("Create needed resources")
	fmt.Println("\tCreate resource group...")
	resourceGroupParameters := resources.Group{
		Location: &location,
	}
	_, err := groupClient.CreateOrUpdate(groupName, resourceGroupParameters)
	onErrorFail(err, "CreateOrUpdate resource group failed")

	go func() {
		fmt.Println("\tStarting to create storage account...")
		accountParameters := storage.AccountCreateParameters{
			Sku: &storage.Sku{
				Name: storage.StandardLRS,
			},
			Location: &location,
			AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
		}
		_, errStorage := accountClient.Create(groupName, accountName, accountParameters, nil)
		onErrorFail(<-errStorage, "Create storage account failed")
		fmt.Println("... storage account created")
	}()

	fmt.Println("\tStarting to create virtual network...")
	vNetName := "vNet"
	vNetParameters := network.VirtualNetwork{
		Location: &location,
		VirtualNetworkPropertiesFormat: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{
				AddressPrefixes: &[]string{"10.0.0.0/16"},
			},
		},
	}
	_, errVnet := vNetClient.CreateOrUpdate(groupName, vNetName, vNetParameters, nil)
	onErrorFail(<-errVnet, "CreateOrUpdate virtual network failed")
	fmt.Println("... virtual network created")

	fmt.Println("\tStarting to create subnet...")
	subnetName := "subnet"
	subnet := network.Subnet{
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("10.0.0.0/24"),
		},
	}
	_, errSubnet := subnetClient.CreateOrUpdate(groupName, vNetName, subnetName, subnet, nil)
	onErrorFail(<-errSubnet, "CreateOrUpdate virtual network failed")
	fmt.Println("... subnet created")

	fmt.Println("\tGet subnet info...")
	subnet, err = subnetClient.Get(groupName, vNetName, subnetName, "")
	onErrorFail(err, "Get subnet failed")

	return &subnet
}

// createVM creates a VM in the provided subnet.
func createVM(vmName, publisher, offer, sku string, subnet *network.Subnet) error {
	publicIPaddress, nicParameters := createPIPandNIC(vmName, subnet)

	fmt.Printf("Create '%s' VM...\n", vmName)
	vm := setVMparameters(vmName, publisher, offer, sku, *nicParameters.ID)
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, vm, nil)
	onErrorFail(<-errChan, "CreateOrUpdate '%s' failed", vmName)

	fmt.Printf("Now you can connect to '%s' VM via 'ssh %s@%s' with password '%s'\n",
		vmName,
		*vm.OsProfile.AdminUsername,
		*publicIPaddress.DNSSettings.Fqdn,
		*vm.OsProfile.AdminPassword)

	return nil
}

// createPIPandNIC creates a public IP address and a network interface in an existing subnet.
// It returns a network interface ready to be used to create a virtual machine.
func createPIPandNIC(machine string, subnet *network.Subnet) (*network.PublicIPAddress, *network.Interface) {
	fmt.Printf("Create PIP and NIC for %s VM...\n", machine)
	IPname := fmt.Sprintf("pip-%s", machine)
	fmt.Printf("\tStarting to create public IP address '%v'...\n", IPname)
	pip := network.PublicIPAddress{
		Location: &location,
		PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
			DNSSettings: &network.PublicIPAddressDNSSettings{
				DomainNameLabel: to.StringPtr(fmt.Sprintf("azuresample-%s", strings.ToLower(machine[:5]))),
			},
		},
	}
	_, errPIP := addressClient.CreateOrUpdate(groupName, IPname, pip, nil)
	onErrorFail(<-errPIP, "CreateOrUpdate '%s' failed", IPname)
	fmt.Printf("... public IP address '%v' created\n", IPname)

	fmt.Println("\tGet IP address info...")
	pip, err := addressClient.Get(groupName, IPname, "")
	onErrorFail(err, "Get '%s' failed", IPname)

	nicName := fmt.Sprintf("nic-%s", machine)
	fmt.Printf("\tStarting to create NIC '%v'...\n", nicName)
	nic := network.Interface{
		Location: &location,
		InterfacePropertiesFormat: &network.InterfacePropertiesFormat{
			IPConfigurations: &[]network.InterfaceIPConfiguration{
				{
					Name: to.StringPtr(fmt.Sprintf("IPconfig-%s", machine)),
					InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
						PublicIPAddress:           &pip,
						PrivateIPAllocationMethod: network.Dynamic,
						Subnet: subnet,
					},
				},
			},
		},
	}
	_, errChan := interfacesClient.CreateOrUpdate(groupName, nicName, nic, nil)
	onErrorFail(<-errChan, "CreateOrUpdate '%s' failed", nicName)
	fmt.Printf("... NIC '%v' created\n", nicName)

	fmt.Println("\tGet NIC info...")
	nic, err = interfacesClient.Get(groupName, nicName, "")
	onErrorFail(err, "Get '%s' failed", nicName)

	return &pip, &nic
}

// setVMparameters builds the VirtualMachine argument for creating or updating a VM.
func setVMparameters(vmName, publisher, offer, sku, nicID string) compute.VirtualMachine {
	return compute.VirtualMachine{
		Location: &location,
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: compute.StandardDS1,
			},
			StorageProfile: &compute.StorageProfile{
				ImageReference: &compute.ImageReference{
					Publisher: &publisher,
					Offer:     &offer,
					Sku:       &sku,
					Version:   to.StringPtr("latest"),
				},
				OsDisk: &compute.OSDisk{
					Name: to.StringPtr("osDisk"),
					Vhd: &compute.VirtualHardDisk{
						URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, vmName)),
					},
					CreateOption: compute.FromImage,
				},
			},
			OsProfile: &compute.OSProfile{
				ComputerName:  &vmName,
				AdminUsername: to.StringPtr("notadmin"),
				AdminPassword: to.StringPtr("Pa$$w0rd1975"),
			},
			NetworkProfile: &compute.NetworkProfile{
				NetworkInterfaces: &[]compute.NetworkInterfaceReference{
					{
						ID: &nicID,
						NetworkInterfaceReferenceProperties: &compute.NetworkInterfaceReferenceProperties{
							Primary: to.BoolPtr(true),
						},
					},
				},
			},
		},
	}
}

// vmOperations performs simple VM operations.
func vmOperations(vmName string) {
	fmt.Printf("Performing various operations on '%s' VM\n", vmName)
	vm := getVM(vmName)

	updateVM(vmName, vm)
	attachDataDisk(vmName, vm)
	detachDataDisks(vmName, vm)
	updateOSdiskSize(vmName, vm)
	startVM(vmName)
	restartVM(vmName)
	stopVM(vmName)
}

func getVM(vmName string) *compute.VirtualMachine {
	fmt.Printf("Get VM '%s' by name\n", vmName)
	vm, err := vmClient.Get(groupName, vmName, compute.InstanceView)
	onErrorFail(err, "Get failed")
	printVM(vm)
	return &vm
}

func updateVM(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Tag VM (via CreateOrUpdate operation)")
	vm.Tags = &(map[string]*string{
		"who rocks": to.StringPtr("golang"),
		"where":     to.StringPtr("on azure"),
	})
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, "CreateOrUpdate '%s' failed", vmName)
}

func attachDataDisk(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Attach data disk (via CreateOrUpdate operation)")
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
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, "CreateOrUpdate '%s' failed", vmName)
}

func detachDataDisks(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Detach data disks (via CreateOrUpdate operation)")
	vm.StorageProfile.DataDisks = &[]compute.DataDisk{}
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, "CreateOrUpdate '%s' failed", vmName)
}

func updateOSdiskSize(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Update OS disk size (via Deallocate and CreateOrUpdate operations)")
	if vm.StorageProfile.OsDisk.DiskSizeGB == nil {
		vm.StorageProfile.OsDisk.DiskSizeGB = to.Int32Ptr(0)
	}

	_, errChan := vmClient.Deallocate(groupName, vmName, nil)
	onErrorFail(<-errChan, "Deallocate '%s' failed", vmName)

	if *vm.StorageProfile.OsDisk.DiskSizeGB <= 0 {
		*vm.StorageProfile.OsDisk.DiskSizeGB = 256
	}
	*vm.StorageProfile.OsDisk.DiskSizeGB += 10

	_, errChan = vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, "CreateOrUpdate '%s' failed", vmName)
}

func startVM(vmName string) {
	fmt.Println("Start VM...")
	_, errChan := vmClient.Start(groupName, vmName, nil)
	onErrorFail(<-errChan, "Start '%s' failed", vmName)
}

func restartVM(vmName string) {
	fmt.Println("Restart VM...")
	_, errChan := vmClient.Restart(groupName, vmName, nil)
	onErrorFail(<-errChan, "Restart '%s' failed", vmName)
}

func stopVM(vmName string) {
	fmt.Println("Stop VM...")
	_, errChan := vmClient.PowerOff(groupName, vmName, nil)
	onErrorFail(<-errChan, "Stop '%s' failed", vmName)
}

func listVMs() {
	fmt.Println("List VMs in subscription...")
	list, err := vmClient.ListAll()
	onErrorFail(err, "ListAll failed")

	if list.Value != nil && len(*list.Value) > 0 {
		fmt.Println("VMs in subscription")
		for _, vm := range *list.Value {
			printVM(vm)
		}
	} else {
		fmt.Println("There are no VMs in this subscription")
	}
}

func deleteVM(vmName string) {
	fmt.Printf("Delete '%s' virtual machine...\n", vmName)
	_, errChan := vmClient.Delete(groupName, vmName, nil)
	err := <-errChan
	onErrorFail(err, "Delete '%s' failed", vmName)
}

// printVM prints basic info about a Virtual Machine.
func printVM(vm compute.VirtualMachine) {
	tags := "\n"
	if vm.Tags == nil {
		tags += "\t\tNo tags yet\n"
	} else {
		for k, v := range *vm.Tags {
			tags += fmt.Sprintf("\t\t%s = %s\n", k, *v)
		}
	}
	fmt.Printf("Virtual machine '%s'\n", *vm.Name)
	elements := map[string]interface{}{
		"ID":       *vm.ID,
		"Type":     *vm.Type,
		"Location": *vm.Location,
		"Tags":     tags}
	for k, v := range elements {
		fmt.Printf("\t%s: %s\n", k, v)
	}
}

// getEnvVarOrExit returns the value of specified environment variable or terminates if it's not defined.
func getEnvVarOrExit(varName string) string {
	value := os.Getenv(varName)
	if value == "" {
		fmt.Printf("Missing environment variable %s\n", varName)
		os.Exit(1)
	}

	return value
}

// onErrorFail prints a failure message and exits the program if err is not nil.
func onErrorFail(err error, message string, a ...interface{}) {
	if err != nil {
		fmt.Printf("%s: %s\n", fmt.Sprintf(message, a), err)
		os.Exit(1)
	}
}

func createClients(subscriptionID string, authorizer *autorest.BearerAuthorizer) {
	groupClient = resources.NewGroupsClient(subscriptionID)
	groupClient.Authorizer = authorizer

	accountClient = storage.NewAccountsClient(subscriptionID)
	accountClient.Authorizer = authorizer

	vNetClient = network.NewVirtualNetworksClient(subscriptionID)
	vNetClient.Authorizer = authorizer

	subnetClient = network.NewSubnetsClient(subscriptionID)
	subnetClient.Authorizer = authorizer

	addressClient = network.NewPublicIPAddressesClient(subscriptionID)
	addressClient.Authorizer = authorizer

	interfacesClient = network.NewInterfacesClient(subscriptionID)
	interfacesClient.Authorizer = authorizer

	vmClient = compute.NewVirtualMachinesClient(subscriptionID)
	vmClient.Authorizer = authorizer
}
