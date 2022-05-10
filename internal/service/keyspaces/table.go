package keyspaces

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/keyspaces"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/internal/verify"
)

func ResourceTable() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceTableCreate,
		ReadWithoutTimeout:   resourceTableRead,
		UpdateWithoutTimeout: resourceTableUpdate,
		DeleteWithoutTimeout: resourceTableDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Update: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		CustomizeDiff: verify.SetTagsDiff,

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"keyspace_name": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
				ValidateFunc: validation.All(
					validation.StringLenBetween(1, 48),
					validation.StringMatch(
						regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_]{1,47}$`),
						"The name must consist of alphanumerics and underscores.",
					),
				),
			},
			"table_name": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
				ValidateFunc: validation.All(
					validation.StringLenBetween(1, 48),
					validation.StringMatch(
						regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_]{1,47}$`),
						"The name must consist of alphanumerics and underscores.",
					),
				),
			},
			"tags":     tftags.TagsSchema(),
			"tags_all": tftags.TagsSchemaComputed(),
		},
	}
}

func resourceTableCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).KeyspacesConn
	defaultTagsConfig := meta.(*conns.AWSClient).DefaultTagsConfig
	tags := defaultTagsConfig.MergeTags(tftags.New(d.Get("tags").(map[string]interface{})))

	keyspaceName := d.Get("keyspace_name").(string)
	tableName := d.Get("table_name").(string)
	id := TableCreateResourceID(keyspaceName, tableName)
	input := &keyspaces.CreateTableInput{
		KeyspaceName: aws.String(keyspaceName),
		TableName:    aws.String(tableName),
	}

	if tags := Tags(tags.IgnoreAWS()); len(tags) > 0 {
		// The Keyspaces API requires that when Tags is set, it's non-empty.
		input.Tags = tags
	}

	log.Printf("[DEBUG] Creating Keyspaces Table: %s", input)
	_, err := conn.CreateTableWithContext(ctx, input)

	if err != nil {
		return diag.Errorf("creating Keyspaces Table (%s): %s", id, err)
	}

	d.SetId(id)

	if _, err := waitTableCreated(ctx, conn, keyspaceName, tableName, d.Timeout(schema.TimeoutCreate)); err != nil {
		return diag.Errorf("waiting for Keyspaces Table (%s) create: %s", d.Id(), err)
	}

	return resourceTableRead(ctx, d, meta)
}

func resourceTableRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).KeyspacesConn
	defaultTagsConfig := meta.(*conns.AWSClient).DefaultTagsConfig
	ignoreTagsConfig := meta.(*conns.AWSClient).IgnoreTagsConfig

	keyspaceName, tableName, err := TableParseResourceID(d.Id())

	if err != nil {
		return diag.FromErr(err)
	}

	table, err := FindTableByTwoPartKey(ctx, conn, keyspaceName, tableName)

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] Keyspaces Table (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return diag.Errorf("reading Keyspaces Table (%s): %s", d.Id(), err)
	}

	d.Set("arn", table.ResourceArn)
	d.Set("keyspace_name", table.KeyspaceName)
	d.Set("table_name", table.TableName)

	// TODO More attributes.

	tags, err := ListTags(conn, d.Get("arn").(string))

	if err != nil {
		return diag.Errorf("listing tags for Keyspaces Table (%s): %s", d.Id(), err)
	}

	tags = tags.IgnoreAWS().IgnoreConfig(ignoreTagsConfig)

	//lintignore:AWSR002
	if err := d.Set("tags", tags.RemoveDefaultConfig(defaultTagsConfig).Map()); err != nil {
		return diag.Errorf("setting tags: %s", err)
	}

	if err := d.Set("tags_all", tags.Map()); err != nil {
		return diag.Errorf("setting tags_all: %s", err)
	}

	return nil
}

func resourceTableUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).KeyspacesConn

	keyspaceName, tableName, err := TableParseResourceID(d.Id())

	if err != nil {
		return diag.FromErr(err)
	}

	if d.HasChangesExcept("tags", "tags_all") {
		input := &keyspaces.UpdateTableInput{
			KeyspaceName: aws.String(keyspaceName),
			TableName:    aws.String(tableName),
		}

		log.Printf("[DEBUG] Updating Keyspaces Table: %s", input)
		_, err := conn.UpdateTableWithContext(ctx, input)

		if err != nil {
			return diag.Errorf("updating Keyspaces Table (%s): %s", d.Id(), err)
		}

		if _, err := waitTableUpdated(ctx, conn, keyspaceName, tableName, d.Timeout(schema.TimeoutUpdate)); err != nil {
			return diag.Errorf("waiting for Keyspaces Table (%s) update: %s", d.Id(), err)
		}
	}

	if d.HasChange("tags_all") {
		o, n := d.GetChange("tags_all")

		if err := UpdateTags(conn, d.Get("arn").(string), o, n); err != nil {
			return diag.Errorf("updating Keyspaces Table (%s) tags: %s", d.Id(), err)
		}
	}

	return resourceTableRead(ctx, d, meta)
}

func resourceTableDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).KeyspacesConn

	keyspaceName, tableName, err := TableParseResourceID(d.Id())

	if err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Deleting Keyspaces Table: (%s)", d.Id())
	_, err = conn.DeleteTableWithContext(ctx, &keyspaces.DeleteTableInput{
		KeyspaceName: aws.String(keyspaceName),
		TableName:    aws.String(tableName),
	})

	if tfawserr.ErrCodeEquals(err, keyspaces.ErrCodeResourceNotFoundException) {
		return nil
	}

	if err != nil {
		return diag.Errorf("deleting Keyspaces Table (%s): %s", d.Id(), err)
	}

	if _, err := waitTableDeleted(ctx, conn, keyspaceName, tableName, d.Timeout(schema.TimeoutDelete)); err != nil {
		return diag.Errorf("waiting for Keyspaces Table (%s) delete: %s", d.Id(), err)
	}

	return nil
}

const tableIDSeparator = "/"

func TableCreateResourceID(keyspaceName, tableName string) string {
	parts := []string{keyspaceName, tableName}
	id := strings.Join(parts, tableIDSeparator)

	return id
}

func TableParseResourceID(id string) (string, string, error) {
	parts := strings.Split(id, tableIDSeparator)

	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("unexpected format for ID (%[1]s), expected KEYSPACE-NAME%[2]sTABLE-NAME", id, tableIDSeparator)
}

func statusTable(ctx context.Context, conn *keyspaces.Keyspaces, keyspaceName, tableName string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := FindTableByTwoPartKey(ctx, conn, keyspaceName, tableName)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, aws.StringValue(output.Status), nil
	}
}

func waitTableCreated(ctx context.Context, conn *keyspaces.Keyspaces, keyspaceName, tableName string, timeout time.Duration) (*keyspaces.GetTableOutput, error) {
	stateConf := &resource.StateChangeConf{
		Pending: []string{keyspaces.TableStatusCreating},
		Target:  []string{keyspaces.TableStatusActive},
		Refresh: statusTable(ctx, conn, keyspaceName, tableName),
		Timeout: timeout,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*keyspaces.GetTableOutput); ok {
		return output, err
	}

	return nil, err
}

func waitTableDeleted(ctx context.Context, conn *keyspaces.Keyspaces, keyspaceName, tableName string, timeout time.Duration) (*keyspaces.GetTableOutput, error) {
	stateConf := &resource.StateChangeConf{
		Pending: []string{keyspaces.TableStatusDeleting},
		Target:  []string{},
		Refresh: statusTable(ctx, conn, keyspaceName, tableName),
		Timeout: timeout,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*keyspaces.GetTableOutput); ok {
		return output, err
	}

	return nil, err
}

func waitTableUpdated(ctx context.Context, conn *keyspaces.Keyspaces, keyspaceName, tableName string, timeout time.Duration) (*keyspaces.GetTableOutput, error) {
	stateConf := &resource.StateChangeConf{
		Pending: []string{keyspaces.TableStatusUpdating},
		Target:  []string{keyspaces.TableStatusActive},
		Refresh: statusTable(ctx, conn, keyspaceName, tableName),
		Timeout: timeout,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*keyspaces.GetTableOutput); ok {
		return output, err
	}

	return nil, err
}
