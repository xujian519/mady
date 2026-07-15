package domains

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRegistry_RegisterAndLookup(t *testing.T) {
	dir := t.TempDir()
	tmpDir := t.TempDir()

	reg, err := NewProjectRegistry(dir)
	if err != nil {
		t.Fatalf("NewProjectRegistry: %v", err)
	}

	rec := ProjectRecord{
		ProjectID: "proj_test_001",
		Domain:    DomainPatent,
		Alias:     "测试案件",
		RootPath:  tmpDir,
	}

	if err := reg.Register(rec); err != nil {
		t.Fatalf("Register: %v", err)
	}

	loaded, ok := reg.Lookup("proj_test_001")
	if !ok {
		t.Fatal("Lookup should find registered record")
	}
	if loaded.Alias != "测试案件" {
		t.Errorf("Alias = %q, want '测试案件'", loaded.Alias)
	}
	if loaded.Status != "active" {
		t.Errorf("Status = %q, want 'active'", loaded.Status)
	}
}

func TestProjectRegistry_RegisterInvalidPath(t *testing.T) {
	dir := t.TempDir()

	reg, err := NewProjectRegistry(dir)
	if err != nil {
		t.Fatalf("NewProjectRegistry: %v", err)
	}

	rec := ProjectRecord{
		ProjectID: "proj_invalid",
		Domain:    DomainPatent,
		RootPath:  "/nonexistent/path/xyz789",
	}

	if err := reg.Register(rec); err == nil {
		t.Fatal("expected error for invalid RootPath")
	}
}

func TestProjectRegistry_DuplicateRootPath(t *testing.T) {
	dir := t.TempDir()
	tmpDir := t.TempDir()

	reg, _ := NewProjectRegistry(dir)

	r1 := ProjectRecord{ProjectID: "proj_001", Domain: DomainPatent, Alias: "案件一", RootPath: tmpDir}
	r2 := ProjectRecord{ProjectID: "proj_002", Domain: DomainLegal, Alias: "案件二", RootPath: tmpDir}

	if err := reg.Register(r1); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := reg.Register(r2); err == nil {
		t.Fatal("expected error for duplicate RootPath")
	}
}

func TestProjectRegistry_PersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	tmpDir := t.TempDir()

	reg1, _ := NewProjectRegistry(dir)
	rec := ProjectRecord{ProjectID: "proj_persist", Domain: DomainPatent, Alias: "持久化测试", RootPath: tmpDir}
	if err := reg1.Register(rec); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 新实例重新加载
	reg2, err := NewProjectRegistry(dir)
	if err != nil {
		t.Fatalf("New second registry: %v", err)
	}

	loaded, ok := reg2.Lookup("proj_persist")
	if !ok {
		t.Fatal("record should persist across instances")
	}
	if loaded.Domain != DomainPatent {
		t.Errorf("Domain = %q, want %q", loaded.Domain, DomainPatent)
	}
}

func TestProjectRegistry_Delete(t *testing.T) {
	dir := t.TempDir()
	tmpDir := t.TempDir()

	reg, _ := NewProjectRegistry(dir)
	rec := ProjectRecord{ProjectID: "proj_del", Domain: DomainPatent, RootPath: tmpDir}
	reg.Register(rec)

	if err := reg.Delete("proj_del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := reg.Lookup("proj_del"); ok {
		t.Fatal("record should be deleted")
	}
}

func TestProjectRegistry_List(t *testing.T) {
	dir := t.TempDir()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	reg, _ := NewProjectRegistry(dir)
	reg.Register(ProjectRecord{ProjectID: "proj_a", Domain: DomainPatent, RootPath: dir1})
	reg.Register(ProjectRecord{ProjectID: "proj_b", Domain: DomainLegal, RootPath: dir2})

	all := reg.List()
	if len(all) != 2 {
		t.Fatalf("List returned %d records, want 2", len(all))
	}
}

func TestProjectRegistry_Touch(t *testing.T) {
	dir := t.TempDir()
	tmpDir := t.TempDir()

	reg, _ := NewProjectRegistry(dir)
	reg.Register(ProjectRecord{ProjectID: "proj_touch", Domain: DomainPatent, RootPath: tmpDir})

	before, _ := reg.Lookup("proj_touch")
	reg.Touch("proj_touch")
	after, _ := reg.Lookup("proj_touch")

	if !after.LastAccessed.After(before.LastAccessed) {
		t.Error("Touch should update LastAccessed")
	}
}

func TestProjectRegistry_RefreshStatus_Active(t *testing.T) {
	dir := t.TempDir()
	tmpDir := t.TempDir()

	reg, _ := NewProjectRegistry(dir)
	reg.Register(ProjectRecord{ProjectID: "proj_active", Domain: DomainPatent, RootPath: tmpDir})

	reg.RefreshStatus()

	rec, _ := reg.Lookup("proj_active")
	if rec.Status != "active" {
		t.Errorf("Status = %q, want 'active'", rec.Status)
	}
}

func TestProjectRegistry_SaveAndLoadMeta(t *testing.T) {
	dir := t.TempDir()
	tmpDir := t.TempDir()

	reg, _ := NewProjectRegistry(dir)
	rec := ProjectRecord{ProjectID: "proj_meta", Domain: DomainPatent, RootPath: tmpDir}
	reg.Register(rec)

	meta := &ProjectMeta{
		ProjectID:  "proj_meta",
		Domain:     DomainPatent,
		Alias:      "meta测试",
		RootPath:   tmpDir,
		MatterType: "发明专利申请",
		ClientName: "测试客户",
		Status:     "active",
	}

	if err := reg.SaveMeta("proj_meta", meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	loaded, err := reg.LoadMeta("proj_meta")
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if loaded.Alias != "meta测试" {
		t.Errorf("Alias = %q", loaded.Alias)
	}
	if loaded.MatterType != "发明专利申请" {
		t.Errorf("MatterType = %q", loaded.MatterType)
	}
}

func TestProjectRegistry_MetaDoesNotExist(t *testing.T) {
	dir := t.TempDir()

	reg, _ := NewProjectRegistry(dir)
	reg.Register(ProjectRecord{ProjectID: "proj_nometa", Domain: DomainPatent, RootPath: t.TempDir()})

	_, err := reg.LoadMeta("proj_nometa")
	if err == nil {
		t.Fatal("expected error for missing meta")
	}
}

func TestValidateProjectPath(t *testing.T) {
	// 正常目录
	tmpDir := t.TempDir()
	if err := ValidateProjectPath(tmpDir); err != nil {
		t.Fatalf("ValidateProjectPath on existing dir: %v", err)
	}

	// 不存在路径
	if err := ValidateProjectPath("/nonexistent_path_xyz"); err == nil {
		t.Error("expected error for nonexistent path")
	}

	// 文件而非目录
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProjectPath(tmpFile); err == nil {
		t.Error("expected error for file path")
	}

	// 符号链接指向目录应被解析并通过
	linkDir := filepath.Join(tmpDir, "link_dir")
	if err := os.Symlink(tmpDir, linkDir); err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}
	if err := ValidateProjectPath(linkDir); err != nil {
		t.Errorf("ValidateProjectPath on symlink to dir: %v", err)
	}

	// 符号链接指向不存在路径应被拒绝
	brokenLink := filepath.Join(tmpDir, "broken_link")
	if err := os.Symlink(filepath.Join(tmpDir, "does_not_exist"), brokenLink); err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}
	if err := ValidateProjectPath(brokenLink); err == nil {
		t.Error("expected error for broken symlink")
	}
}

func TestNewProjectRegistryOrEmpty(t *testing.T) {
	// 不存在的目录
	reg := NewProjectRegistryOrEmpty("/nonexistent_registry_dir")
	if reg == nil {
		t.Fatal("NewProjectRegistryOrEmpty should not return nil")
	}
	if len(reg.List()) != 0 {
		t.Fatal("should start empty")
	}
}
