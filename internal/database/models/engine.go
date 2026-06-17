package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EngineType represents a type of inference engine (e.g. vllm, sglang).
type EngineType struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug        string    `gorm:"uniqueIndex;size:128;not null;column:slug"`
	Name        string    `gorm:"size:256;column:name"`
	Description string    `gorm:"type:text;default:'';column:description"`
	CreatedAt   time.Time `gorm:"autoCreateTime;column:created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime;column:updated_at"`
}

// TableName returns the database table name for EngineType.
func (EngineType) TableName() string { return "engine_types" }

// BeforeCreate generates a UUID for new EngineType records.
func (e *EngineType) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// EngineVersion represents a specific version of an inference engine with its Docker recipe.
type EngineVersion struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug               string    `gorm:"size:128;not null;column:slug"`
	EngineTypeSlug     string    `gorm:"size:128;not null;column:engine_type_slug;uniqueIndex:idx_engine_version_type_slug_unique"`
	Version            string    `gorm:"size:32;not null;column:version;uniqueIndex:idx_engine_version_type_slug_unique"`
	ContainerName      string    `gorm:"size:128;column:container_name"`
	Image              string    `gorm:"size:500;not null;column:image"`
	Entrypoint         string    `gorm:"type:text;default:'';column:entrypoint"`
	IsDefault          bool      `gorm:"type:boolean;default:false;column:is_default"`
	IsLatest           bool      `gorm:"type:boolean;default:true;column:is_latest"`
	EnvironmentJSON    string    `gorm:"type:text;column:environment_json"`
	VolumesJSON        string    `gorm:"type:text;column:volumes_json"`
	EnableLogging      bool      `gorm:"type:boolean;default:false;column:enable_logging"`
	SyslogAddress      string    `gorm:"size:255;default:'';column:syslog_address"`
	SyslogFacility     string    `gorm:"size:64;default:'local3';column:syslog_facility"`
	DeployEnableNvidia bool      `gorm:"type:boolean;default:false;column:deploy_enable_nvidia"`
	DeployGPUCount     string    `gorm:"size:16;default:'';column:deploy_gpu_count"`
	CommandArgs        string    `gorm:"type:text;default:'';column:command_args"`
	HealthcheckJSON    string    `gorm:"type:text;column:healthcheck_json"`
	UlimitsJSON        string    `gorm:"type:text;column:ulimits_json"`
	IPC                string    `gorm:"size:32;default:'';column:ipc"`
	CreatedAt          time.Time `gorm:"autoCreateTime;column:created_at"`
	UpdatedAt          time.Time `gorm:"autoUpdateTime;column:updated_at"`
}

// TableName returns the database table name for EngineVersion.
func (EngineVersion) TableName() string { return "engine_versions" }

// BeforeCreate generates a UUID for new EngineVersion records.
func (e *EngineVersion) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// GetEnvironment parses EnvironmentJSON into a map[string]string.
// Returns an empty map (not nil) when the JSON is empty or invalid.
func (e *EngineVersion) GetEnvironment() map[string]string {
	if e.EnvironmentJSON == "" {
		return make(map[string]string)
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(e.EnvironmentJSON), &env); err != nil {
		return make(map[string]string)
	}
	return env
}

// SetEnvironment serializes a map[string]string to JSON and stores it in EnvironmentJSON.
func (e *EngineVersion) SetEnvironment(env map[string]string) error {
	if env == nil || len(env) == 0 {
		e.EnvironmentJSON = ""
		return nil
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	e.EnvironmentJSON = string(b)
	return nil
}

// GetVolumes parses VolumesJSON into a map[string]string.
// Returns an empty map (not nil) when the JSON is empty or invalid.
func (e *EngineVersion) GetVolumes() map[string]string {
	if e.VolumesJSON == "" {
		return make(map[string]string)
	}
	var vols map[string]string
	if err := json.Unmarshal([]byte(e.VolumesJSON), &vols); err != nil {
		return make(map[string]string)
	}
	return vols
}

// SetVolumes serializes a map[string]string to JSON and stores it in VolumesJSON.
func (e *EngineVersion) SetVolumes(vols map[string]string) error {
	if vols == nil || len(vols) == 0 {
		e.VolumesJSON = ""
		return nil
	}
	b, err := json.Marshal(vols)
	if err != nil {
		return err
	}
	e.VolumesJSON = string(b)
	return nil
}

// GetCommandArgs parses CommandArgs JSON array into a []string.
// Returns nil when the JSON is empty or invalid.
func (e *EngineVersion) GetCommandArgs() []string {
	if e.CommandArgs == "" {
		return nil
	}
	var args []string
	if err := json.Unmarshal([]byte(e.CommandArgs), &args); err != nil {
		return nil
	}
	return args
}

// GetHealthcheck parses HealthcheckJSON into a map[string]interface{}.
// Returns an empty map (not nil) when the JSON is empty or invalid.
func (e *EngineVersion) GetHealthcheck() map[string]interface{} {
	if e.HealthcheckJSON == "" {
		return make(map[string]interface{})
	}
	var hc map[string]interface{}
	if err := json.Unmarshal([]byte(e.HealthcheckJSON), &hc); err != nil {
		return make(map[string]interface{})
	}
	return hc
}

// SetHealthcheck serializes a map[string]interface{} to JSON and stores it in HealthcheckJSON.
func (e *EngineVersion) SetHealthcheck(hc map[string]interface{}) error {
	if hc == nil || len(hc) == 0 {
		e.HealthcheckJSON = ""
		return nil
	}
	b, err := json.Marshal(hc)
	if err != nil {
		return err
	}
	e.HealthcheckJSON = string(b)
	return nil
}

// GetUlimits parses UlimitsJSON into a map[string]interface{}.
// Returns an empty map (not nil) when the JSON is empty or invalid.
func (e *EngineVersion) GetUlimits() map[string]interface{} {
	if e.UlimitsJSON == "" {
		return make(map[string]interface{})
	}
	var ul map[string]interface{}
	if err := json.Unmarshal([]byte(e.UlimitsJSON), &ul); err != nil {
		return make(map[string]interface{})
	}
	return ul
}

// SetUlimits serializes a map[string]interface{} to JSON and stores it in UlimitsJSON.
func (e *EngineVersion) SetUlimits(ul map[string]interface{}) error {
	if ul == nil || len(ul) == 0 {
		e.UlimitsJSON = ""
		return nil
	}
	b, err := json.Marshal(ul)
	if err != nil {
		return err
	}
	e.UlimitsJSON = string(b)
	return nil
}

// GetIPC returns the IPC namespace setting (e.g., "host", "share", "none").
// Returns an empty string when not set.
func (e *EngineVersion) GetIPC() string {
	return e.IPC
}

// SetIPC sets the IPC namespace setting.
func (e *EngineVersion) SetIPC(ipc string) {
	e.IPC = ipc
}

// SetCommandArgs serializes a []string to JSON and stores it in CommandArgs.
func (e *EngineVersion) SetCommandArgs(args []string) error {
	if args == nil || len(args) == 0 {
		e.CommandArgs = ""
		return nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	e.CommandArgs = string(b)
	return nil
}
