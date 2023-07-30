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
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
				Description: "The Universally Unique IDentifier (UUID) of the Assignment.",
			},
			"owner": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The Universally Unique IDentifier (UUID) of the Assignment.",
			},
			"groups": {
				Type:          schema.TypeList,
				Optional:      true,
				Elem:          &schema.Schema{Type: schema.TypeString},
				ConflictsWith: []string{"cluster", "controller"},
				Description:   "One or more cluster groups on which you want to apply the configuration. Note that at least one cluster group is required. ",
			},
			"cluster": {
				Type:         schema.TypeString,
				Optional:     true,
				Description:  "ID of the Satellite cluster or Service Cluster that you want to apply the configuration to.",
				RequiredWith: []string{"controller"},
			},
			"svc_cluster": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "ID of the Service Cluster that you applied the configuration to.",
			},
			"sat_cluster": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "ID of the Satellite cluster that applied the configuration to.",
			},
			"config": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Storage Configuration Name or ID.",
			},
			"config_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The Universally Unique IDentifier (UUID) of the Storage Configuration.",
			},
			"config_version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The Storage Configuration Version.",
			},
			"config_version_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The Universally Unique IDentifier (UUID) of the Storage Configuration Version.",
			},
			"assignment_type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The Type of Assignment.",
			},
			"created": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The Time of Creation of the Assignment.",
			},
			"rollout_success_count": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The Rollout Success Count of the Assignment.",
			},
			"rollout_error_count": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The Rollout Error Count of the Assignment.",
			},
			"is_assignment_upgrade_available": {
				Type:        schema.TypeBool,
				Computed:    true,
				ForceNew:    false,
				Description: "Whether an Upgrade is Available for the Assignment.",
			},
			"update_config_version": {
				Type:        schema.TypeBool,
				Required:    true,
				Default:     false,
				Description: "Updating an assignment to the latest available storage configuration version.",
			},
			"controller": {
				Type:          schema.TypeString,
				Optional:      true,
				Description:   "The Name or ID of the Satellite Location.",
				ConflictsWith: []string{"groups"},
				RequiredWith:  []string{"cluster"},
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

	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	uuid := d.Get("uuid").(string)
	controller := d.Get("controller").(string)
	d.Set("controller", controller)

	getAssignmentOptions := &kubernetesserviceapiv1.GetAssignmentOptions{
		UUID: &uuid,
	}

	result, _, err := satClient.GetAssignment(getAssignmentOptions)
	if err != nil {
		return fmt.Errorf("[ERROR] Error getting Assignment of UUID %s - %v", uuid, err)
	}

	d.Set("assignment_name", *result.Name)
	d.Set("uuid", *result.UUID)
	d.Set("owner", *result.Owner.Name)
	if result.Groups != nil {
		d.Set("groups", result.Groups)
	}
	if result.Cluster != nil {
		d.Set("cluster", *result.ClusterName)
	}
	if result.SatSvcClusterID != nil {
		d.Set("svc_cluster", *result.SatSvcClusterID)
	}
	if result.Satcluster != nil {
		d.Set("sat_cluster", *result.Satcluster)
	}
	if result.ChannelName != nil {
		d.Set("config", *result.ChannelName)
	}
	if result.ChannelUUID != nil {
		d.Set("config_uuid", *result.ChannelUUID)
	}
	if result.Version != nil {
		d.Set("config_version", *result.Version)
	}
	if result.VersionUUID != nil {
		d.Set("config_version_uuid", *result.VersionUUID)
	}
	if result.SubscriptionType != nil {
		d.Set("assignment_type", *result.SubscriptionType)
	}
	if result.Created != nil {
		d.Set("created", *result.Created)
	}
	if result.IsAssignmentUpgradeAvailable != nil {
		d.Set("isAssignmentUpgradeAvailable", *result.IsAssignmentUpgradeAvailable)
	} else {
		d.Set("isAssignmentUpgradeAvailable", false)
	}
	if result.RolloutStatus != nil {
		d.Set("rollout_success_count", *result.RolloutStatus.SuccessCount)
		d.Set("rollout_error_count", *result.RolloutStatus.ErrorCount)
	}
	d.Set("update_config_version", false)
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
