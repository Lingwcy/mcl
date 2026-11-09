package platform

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	"mcl/internal/core/discovery"
)

// ReadSwiftProfiles never opens a foreign SQLite path. SQLite sees only the
// validated Go-owned image, read-only in memory; no SHM, lock, journal or temp
// file can be created in the selected launcher. No account/command column is
// selected and no external SQL, extension, script or process is accepted.
func ReadSwiftProfiles(ctx context.Context, snapshot []byte, selected string) ([]discovery.SwiftProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(snapshot) < 100 || len(snapshot) > discovery.MaxSQLiteBytes || len(selected) > 255 {
		return nil, errors.New("Swift实例索引快照无效")
	}
	library, err := purego.Dlopen("/usr/lib/libsqlite3.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, errors.New("无法打开系统SQLite实例索引读取器")
	}
	defer purego.Dlclose(library)
	var open func(string, *uintptr, int32, uintptr) int32
	var close func(uintptr) int32
	var deserialize func(uintptr, string, uintptr, int64, int64, uint32) int32
	var prepare func(uintptr, string, int32, *uintptr, uintptr) int32
	var finalize func(uintptr) int32
	var step func(uintptr) int32
	var columnText func(uintptr, int32) uintptr
	var columnBytes func(uintptr, int32) int32
	var columnType func(uintptr, int32) int32
	var limit func(uintptr, int32, int32) int32
	var bindText func(uintptr, int32, string, int32, uintptr) int32
	for _, binding := range []struct {
		name   string
		target any
	}{{"sqlite3_open_v2", &open}, {"sqlite3_close_v2", &close}, {"sqlite3_deserialize", &deserialize}, {"sqlite3_prepare_v2", &prepare}, {"sqlite3_finalize", &finalize}, {"sqlite3_step", &step}, {"sqlite3_column_text", &columnText}, {"sqlite3_column_bytes", &columnBytes}, {"sqlite3_column_type", &columnType}, {"sqlite3_limit", &limit}, {"sqlite3_bind_text", &bindText}} {
		address, e := purego.Dlsym(library, binding.name)
		if e != nil {
			return nil, errors.New("当前系统SQLite缺少受控内存索引接口")
		}
		purego.RegisterFunc(binding.target, address)
	}
	var pin runtime.Pinner
	pin.Pin(&snapshot[0])
	defer pin.Unpin()
	var database uintptr
	if open(":memory:", &database, 0x2|0x4|0x80|0x8000|0x40000, 0) != 0 {
		if database != 0 {
			close(database)
		}
		return nil, errors.New("无法创建内存实例索引")
	}
	defer close(database)
	size := int64(len(snapshot))
	if deserialize(database, "main", uintptr(unsafe.Pointer(&snapshot[0])), size, size, 4) != 0 {
		return nil, errors.New("无法读取Swift实例索引快照")
	}
	limit(database, 0, discovery.MaxDocumentBytes)
	limit(database, 1, 16384)
	limit(database, 7, 0)
	readText := func(statement uintptr, index int32, maximum int) (string, error) {
		kind := columnType(statement, index)
		if kind == 5 {
			return "", nil
		}
		if kind != 3 {
			return "", errors.New("Swift实例索引字段类型不受支持")
		}
		length := columnBytes(statement, index)
		if length < 0 || int(length) > maximum {
			return "", errors.New("Swift实例索引字段超过读取边界")
		}
		if length == 0 {
			return "", nil
		}
		address := columnText(statement, index)
		if address == 0 {
			return "", errors.New("Swift实例索引文本不可读取")
		}
		return string(unsafe.Slice((*byte)(unsafe.Pointer(address)), int(length))), nil
	}
	makeStatement := func(query string) (uintptr, error) {
		var statement uintptr
		if prepare(database, query, -1, &statement, 0) != 0 {
			if statement != 0 {
				finalize(statement)
			}
			return 0, errors.New("Swift实例索引格式不受支持")
		}
		return statement, nil
	}
	schema, err := makeStatement("SELECT type,sql FROM sqlite_schema WHERE name='game_versions'")
	if err != nil {
		return nil, err
	}
	if step(schema) != 100 {
		finalize(schema)
		return nil, errors.New("Swift索引缺少game_versions实例表")
	}
	kind, e := readText(schema, 0, 32)
	definition, dErr := readText(schema, 1, 16384)
	finalize(schema)
	fields := strings.Fields(strings.ToUpper(definition))
	if e != nil || dErr != nil || kind != "table" || len(fields) < 2 || fields[0] != "CREATE" || fields[1] != "TABLE" {
		return nil, errors.New("Swift索引不是受支持的普通实例表")
	}
	query := "SELECT id,working_path,game_name,json_extract(data_json,'$.gameVersion'),json_extract(data_json,'$.modLoader'),json_extract(data_json,'$.modVersion'),json_extract(data_json,'$.mainClass') FROM game_versions"
	if selected != "" {
		query += " WHERE game_name=?1"
	}
	query += " LIMIT 2049"
	statement, err := makeStatement(query)
	if err != nil {
		return nil, err
	}
	defer finalize(statement)
	if selected != "" && bindText(statement, 1, selected, -1, ^uintptr(0)) != 0 {
		return nil, errors.New("无法选择Swift实例记录")
	}
	var rows []discovery.SwiftProfile
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		code := step(statement)
		if code == 101 {
			break
		}
		if code != 100 {
			return nil, errors.New("Swift实例索引损坏或版本不受支持")
		}
		if len(rows) >= discovery.MaxCandidates {
			return nil, errors.New("Swift实例索引超过2048项，请直接选择所需的实例目录")
		}
		var row discovery.SwiftProfile
		for index, target := range []*string{&row.ID, &row.WorkingPath, &row.Name, &row.GameVersion, &row.Loader, &row.LoaderVersion, &row.MainClass} {
			maximum := 512
			if index == 1 {
				maximum = 4096
			}
			value, err := readText(statement, int32(index), maximum)
			if err != nil {
				return nil, err
			}
			*target = value
		}
		rows = append(rows, row)
	}
	runtime.KeepAlive(snapshot)
	return rows, ctx.Err()
}
