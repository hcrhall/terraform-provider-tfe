package tfe

import (
	"fmt"
	"log"
	"strings"

	tfe "github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
)

func resourceTFETeamAccess() *schema.Resource {
	return &schema.Resource{
		Create: resourceTFETeamAccessCreate,
		Read:   resourceTFETeamAccessRead,
		Update: resourceTFETeamAccessUpdate,
		Delete: resourceTFETeamAccessDelete,
		Importer: &schema.ResourceImporter{
			State: resourceTFETeamAccessImporter,
		},

		CustomizeDiff: setCustomOrComputedPermissions,
		SchemaVersion: 1,
		StateUpgraders: []schema.StateUpgrader{
			{
				Type:    resourceTfeTeamAccessResourceV0().CoreConfigSchema().ImpliedType(),
				Upgrade: resourceTfeTeamAccessStateUpgradeV0,
				Version: 0,
			},
		},

		Schema: map[string]*schema.Schema{
			"access": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ConflictsWith: []string{"permissions"},
				ValidateFunc: validation.StringInSlice(
					[]string{
						string(tfe.AccessAdmin),
						string(tfe.AccessRead),
						string(tfe.AccessPlan),
						string(tfe.AccessWrite),
					},
					false,
				),
			},

			"permissions": {
				Type:          schema.TypeList,
				MaxItems:      1,
				Optional:      true,
				Computed:      true,
				ConflictsWith: []string{"access"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"runs": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ValidateFunc: validation.StringInSlice(
								[]string{
									string(tfe.RunsPermissionRead),
									string(tfe.RunsPermissionPlan),
									string(tfe.RunsPermissionApply),
								},
								false,
							),
						},

						"variables": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ValidateFunc: validation.StringInSlice(
								[]string{
									string(tfe.VariablesPermissionNone),
									string(tfe.VariablesPermissionRead),
									string(tfe.VariablesPermissionWrite),
								},
								false,
							),
						},

						"state_versions": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ValidateFunc: validation.StringInSlice(
								[]string{
									string(tfe.StateVersionsPermissionNone),
									string(tfe.StateVersionsPermissionReadOutputs),
									string(tfe.StateVersionsPermissionRead),
									string(tfe.StateVersionsPermissionWrite),
								},
								false,
							),
						},

						"sentinel_mocks": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ValidateFunc: validation.StringInSlice(
								[]string{
									string(tfe.SentinelMocksPermissionNone),
									string(tfe.SentinelMocksPermissionRead),
								},
								false,
							),
						},

						"workspace_locking": {
							Type:     schema.TypeBool,
							Optional: true,
							Computed: true,
						},
					},
				},
			},

			"team_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"workspace_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringMatch(
					workspaceIdRegexp,
					"must be the workspace's external_id",
				),
			},
		},
	}
}

func resourceTFETeamAccessCreate(d *schema.ResourceData, meta interface{}) error {
	tfeClient := meta.(*tfe.Client)

	// Get the access level
	access := d.Get("access").(string)

	// Get the workspace
	workspaceID := d.Get("workspace_id").(string)
	ws, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf(
			"Error retrieving workspace %s: %v", workspaceID, err)
	}

	// Get the team.
	teamID := d.Get("team_id").(string)
	tm, err := tfeClient.Teams.Read(ctx, teamID)
	if err != nil {
		return fmt.Errorf("Error retrieving team %s: %v", teamID, err)
	}

	// Create a new options struct.
	options := tfe.TeamAccessAddOptions{
		Access:    tfe.Access(tfe.AccessType(access)),
		Team:      tm,
		Workspace: ws,
	}

	if v, ok := d.GetOk("permissions"); ok {
		permissions := v.([]interface{})[0].(map[string]interface{})

		if permissions["runs"] != "" {
			options.Runs = tfe.RunsPermission(
				tfe.RunsPermissionType(
					permissions["runs"].(string),
				),
			)
		}
		if permissions["variables"] != "" {
			options.Variables = tfe.VariablesPermission(tfe.VariablesPermissionType(permissions["variables"].(string)))
		}
		if permissions["state_versions"] != "" {
			options.StateVersions = tfe.StateVersionsPermission(tfe.StateVersionsPermissionType(permissions["state_versions"].(string)))
		}
		if permissions["sentinel_mocks"] != "" {
			options.SentinelMocks = tfe.SentinelMocksPermission(tfe.SentinelMocksPermissionType(permissions["sentinel_mocks"].(string)))
		}

		// SDK limitations mean we cannot tell if an optional boolean was set (the zero value is false)
		// This is ok here, as the default is false anyway.
		options.WorkspaceLocking = tfe.Bool(permissions["workspace_locking"].(bool))
	}

	log.Printf("[DEBUG] Give team %s %s access to workspace: %s", tm.Name, access, ws.Name)
	tmAccess, err := tfeClient.TeamAccess.Add(ctx, options)
	if err != nil {
		return fmt.Errorf(
			"Error giving team %s %s access to workspace %s: %v", tm.Name, access, ws.Name, err)
	}

	d.SetId(tmAccess.ID)

	return resourceTFETeamAccessRead(d, meta)
}

func resourceTFETeamAccessRead(d *schema.ResourceData, meta interface{}) error {
	tfeClient := meta.(*tfe.Client)

	log.Printf("[DEBUG] Read configuration of team access: %s", d.Id())
	tmAccess, err := tfeClient.TeamAccess.Read(ctx, d.Id())
	if err != nil {
		if err == tfe.ErrResourceNotFound {
			log.Printf("[DEBUG] Team access %s does no longer exist", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading configuration of team access %s: %v", d.Id(), err)
	}

	// Update config.
	d.Set("access", string(tmAccess.Access))
	permissions := []map[string]interface{}{{
		"runs":              tmAccess.Runs,
		"variables":         tmAccess.Variables,
		"state_versions":    tmAccess.StateVersions,
		"sentinel_mocks":    tmAccess.SentinelMocks,
		"workspace_locking": tmAccess.WorkspaceLocking,
	}}
	if err := d.Set("permissions", permissions); err != nil {
		return fmt.Errorf("error setting permissions for team access %s: %s", d.Id(), err)
	}

	if tmAccess.Team != nil {
		d.Set("team_id", tmAccess.Team.ID)
	} else {
		d.Set("team_id", "")
	}

	return nil
}

// Note: Updates do not occur when switching to or from 'custom' access, instead forcing a new resource.
// See forceNewOnAccessChange for more info.
func resourceTFETeamAccessUpdate(d *schema.ResourceData, meta interface{}) error {
	tfeClient := meta.(*tfe.Client)

	// create an options struct
	options := tfe.TeamAccessUpdateOptions{}

	// Set access level
	access := d.Get("access").(string)
	options.Access = tfe.Access(tfe.AccessType(access))

	if d.HasChange("permissions.0.runs") {
		if v, ok := d.GetOk("permissions.0.runs"); ok {
			options.Runs = tfe.RunsPermission(tfe.RunsPermissionType(v.(string)))
		}
	}

	if d.HasChange("permissions.0.variables") {
		if v, ok := d.GetOk("permissions.0.variables"); ok {
			options.Variables = tfe.VariablesPermission(tfe.VariablesPermissionType(v.(string)))
		}
	}

	if d.HasChange("permissions.0.state_versions") {
		if v, ok := d.GetOk("permissions.0.state_versions"); ok {
			options.StateVersions = tfe.StateVersionsPermission(tfe.StateVersionsPermissionType(v.(string)))
		}
	}

	if d.HasChange("permissions.0.sentinel_mocks") {
		if v, ok := d.GetOk("permissions.0.sentinel_mocks"); ok {
			options.SentinelMocks = tfe.SentinelMocksPermission(tfe.SentinelMocksPermissionType(v.(string)))
		}
	}

	if d.HasChange("permissions.0.workspace_locking") {
		if v, ok := d.GetOkExists("permissions.0.workspace_locking"); ok {
			options.WorkspaceLocking = tfe.Bool(v.(bool))
		}
	}

	log.Printf("[DEBUG] Update team access: %s", d.Id())
	tmAccess, err := tfeClient.TeamAccess.Update(ctx, d.Id(), options)
	if err != nil {
		return fmt.Errorf(
			"Error updating team access %s: %v", d.Id(), err)
	}

	// Update permissions, in the case that they were marked to be recomputed.
	permissions := []map[string]interface{}{{
		"runs":              tmAccess.Runs,
		"variables":         tmAccess.Variables,
		"state_versions":    tmAccess.StateVersions,
		"sentinel_mocks":    tmAccess.SentinelMocks,
		"workspace_locking": tmAccess.WorkspaceLocking,
	}}
	if err := d.Set("permissions", permissions); err != nil {
		return fmt.Errorf("error setting permissions for team access %s: %s", d.Id(), err)
	}

	return nil
}

func resourceTFETeamAccessDelete(d *schema.ResourceData, meta interface{}) error {
	tfeClient := meta.(*tfe.Client)

	log.Printf("[DEBUG] Delete team access: %s", d.Id())
	err := tfeClient.TeamAccess.Remove(ctx, d.Id())
	if err != nil {
		if err == tfe.ErrResourceNotFound {
			return nil
		}
		return fmt.Errorf("Error deleting team access %s: %v", d.Id(), err)
	}

	return nil
}

func resourceTFETeamAccessImporter(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	tfeClient := meta.(*tfe.Client)

	s := strings.SplitN(d.Id(), "/", 3)
	if len(s) != 3 {
		return nil, fmt.Errorf(
			"invalid team access import format: %s (expected <ORGANIZATION>/<WORKSPACE>/<TEAM ACCESS ID>)",
			d.Id(),
		)
	}

	// Set the fields that are part of the import ID.
	workspace_id, err := fetchWorkspaceExternalID(s[0]+"/"+s[1], tfeClient)
	if err != nil {
		return nil, fmt.Errorf(
			"error retrieving workspace %s from organization %s: %v", s[0], s[1], err)
	}
	d.Set("workspace_id", workspace_id)
	d.SetId(s[2])

	return []*schema.ResourceData{d}, nil
}

func setCustomOrComputedPermissions(d *schema.ResourceDiff, meta interface{}) error {
	if _, ok := d.GetOk("access"); ok {
		if d.HasChange("access") {
			_, n := d.GetChange("access")
			new := n.(string)

			if new != "custom" {
				// If access is being changed to an explicit value, and access is NOT 'custom', all permissions
				// will be read-only and computed by the API.
				d.SetNewComputed("permissions")
			}
		} else {
			// If access is present, not being explicitly changed, but permissions are being changed, the user is switching
			// from using a fixed access level (read/plan/write/apply) to a permissions block ('custom' access)
			// Set the access to custom.
			if d.HasChange("permissions.0") {
				d.SetNew("access", tfe.AccessCustom)
			}
		}
	} else {
		// If there's no access set, we must be creating a new resource with
		// a permissions block. Set the access to custom.
		if _, ok := d.GetOk("permissions"); ok {
			d.SetNew("access", tfe.AccessCustom)
		}
	}

	return nil
}
