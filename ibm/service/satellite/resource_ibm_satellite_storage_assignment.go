package satellite

import (
	"fmt"
	"time"

	"github.com/IBM-Cloud/container-services-go-sdk/kubernetesserviceapiv1"
	"github.com/IBM-Cloud/terraform-provider-ibm/ibm/conns"
	"github.com/IBM-Cloud/terraform-provider-ibm/ibm/flex"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ResourceIBMSatelliteStorageAssignment() *schema.Resource {
	return &schema.Resource{
		Create:   resourceIBMContainerStorageAssignmentCreate,
		Read:     resourceIBMContainerStorageAssignmentRead,
		Update:   resourceIBMContainerStorageAssignmentUpdate,
		Delete:   resourceIBMContainerStorageAssignmentDelete,
		Exists:   resourceIBMContainerStorageAssignmentExists,
		Importer: &schema.ResourceImporter{},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"assignment_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the Assignment.",
			},
			"config": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Storage Configuration Name or ID.",
			},
			"cluster": {
				Type:         schema.TypeString,
				Optional:     true,
				Description:  "ID of the Satellite cluster or Service Cluster that you want to apply the configuration to.",
				RequiredWith: []string{"controller", "assignment_name", "config"},
			},
			"controller": {
				Type:          schema.TypeString,
				Optional:      true,
				Description:   "The Name or ID of the Satellite Location.",
				ConflictsWith: []string{"groups"},
				RequiredWith:  []string{"cluster", "assignment_name", "config"},
			},
			"groups": {
				Type:          schema.TypeList,
				RequiredWith:  []string{"assignment_name", "config"},
				Elem:          schema.TypeString,
				ConflictsWith: []string{"cluster", "controller"},
				Description:   "One or more cluster groups on which you want to apply the configuration. Note that at least one cluster group is required. ",
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
				Description: "The Universally Unique IDentifier (UUID) of the Assignment.",
			},
		},
	}
}

func resourceIBMContainerStorageAssignmentCreate(d *schema.ResourceData, meta interface{}) error {

	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	var result *kubernetesserviceapiv1.CreateSubscriptionData

	assignmentOptions := &kubernetesserviceapiv1.CreateAssignmentOptions{}

	if v, ok := d.GetOk("assignment_name"); ok {
		name := v.(string)
		assignmentOptions.Name = &name
	}

	if v, ok := d.GetOk("config"); ok {
		config := v.(string)
		assignmentOptions.Config = &config
	}

	if v, ok := d.GetOk("cluster"); ok {
		cluster := v.(string)
		assignmentOptions.Cluster = &cluster
	}

	if v, ok := d.GetOk("controller"); ok {
		controller := v.(string)
		assignmentOptions.Controller = &controller
	}

	if v, groupsOk := d.GetOk("groups"); groupsOk {
		groups := v.([]interface{})
		assignmentOptions.Groups = flex.ExpandStringList(groups)
		result, _, err = satClient.CreateAssignment(assignmentOptions)
		if err != nil {
			return fmt.Errorf("[ERROR] Error Creating Assignment - %v", err)
		}
	} else {
		result, _, err = satClient.CreateAssignmentByCluster(assignmentOptions)
		if err != nil {
			return fmt.Errorf("[ERROR] Error Creating Assignment by Cluster - %v", err)
		}
	}

	d.Set("uuid", *result.AddSubscription.UUID)
	d.SetId(time.Now().String())

	return resourceIBMContainerStorageAssignmentRead(d, meta)
}

func resourceIBMContainerStorageAssignmentRead(d *schema.ResourceData, meta interface{}) error {

	return nil
}

func resourceIBMContainerStorageAssignmentUpdate(d *schema.ResourceData, meta interface{}) error {

	return resourceIBMContainerStorageAssignmentRead(d, meta)
}

func resourceIBMContainerStorageAssignmentDelete(d *schema.ResourceData, meta interface{}) error {

	return nil
}

func resourceIBMContainerStorageAssignmentExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	return true, nil
}
