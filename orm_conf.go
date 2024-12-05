package lorm

import (
	"bytes"
	"github.com/lontten/lorm/field"
	"github.com/lontten/lorm/utils"
	"github.com/pkg/errors"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

type OrmConf struct {
	//po生成文件目录
	PoDir string
	//是否覆盖，默认true
	IsFileOverride bool

	//作者
	Author string
	//是否开启ActiveRecord模式,默认false
	IsActiveRecord bool

	IdType int

	//表名
	//TableNameFun >  tag > TableNamePrefix
	TableNamePrefix string
	TableNameFun    func(t reflect.Value, dest any) string

	//字段名
	FieldNamePrefix string

	//主键 默认为id
	PrimaryKeyNames   []string
	PrimaryKeyNameFun func(v reflect.Value, dest any) []string

	//多租户
	TenantIdFieldName    string                      //多租户的  租户字段名 空字符串极为不启用多租户
	TenantIdValueFun     func() any                  //租户的id值，获取函数
	TenantIgnoreTableFun func(tableName string) bool //该表是否忽略多租户，true忽略该表，即没有多租户
}

var typeTableNameCache = map[reflect.Type]string{}
var typeTableNameMu sync.Mutex

func getTypeTableName(t reflect.Type, tableNamePrefix string) string {
	s, ok := typeTableNameCache[t]
	if ok {
		return s
	}
	typeTableNameMu.Lock()
	defer typeTableNameMu.Unlock()
	s, ok = typeTableNameCache[t]
	if ok {
		return s
	}

	name := t.String()
	index := strings.LastIndex(name, ".")
	if index > 0 {
		name = name[index+1:]
	}
	name = utils.Camel2Case(name)
	if tableNamePrefix != "" {
		name = tableNamePrefix + name
	}
	typeTableNameCache[t] = name
	return name
}

// 不可缓存
// 获取表名
func (c OrmConf) tableName(v reflect.Value, dest any) string {
	// fun
	tableNameFun := c.TableNameFun
	if tableNameFun != nil {
		return tableNameFun(v, dest)
	}

	// tableName
	n := GetTableName(v)
	if n != nil {
		return *n
	}

	// structName
	t := v.Type()
	name := getTypeTableName(t, c.TableNamePrefix)
	return name
}

// 不可缓存
// 1.默认主键为id，
// 2.可以PrimaryKeyNames设置主键字段名
// 3.通过表名动态设置主键字段名-fn
func (c OrmConf) primaryKeys(v reflect.Value, dest any) []string {
	//fun
	primaryKeyNameFun := c.PrimaryKeyNameFun
	if primaryKeyNameFun != nil {
		return primaryKeyNameFun(v, dest)
	}

	list := GetPrimaryKeyNames(v)
	if len(list) > 0 {
		return list
	}

	// id
	return []string{"id"}
}

// 可缓存
func (c OrmConf) autoIncrements(v reflect.Value) []string {
	return GetAutoIncrements(v)
}

// 可以缓存
//
//	主键Id、ID，都转化为id
//
// tag== ldb:name  可以自定义名字
// tag== core:-  跳过
// 过滤掉首字母小写的字段
// 获取model字段对应的 数据库 字段名
func (c OrmConf) initColumns(t reflect.Type) (columns []string, err error) {

	cMap := make(map[string]int)

	numField := t.NumField()
	var num = 0
	for i := 0; i < numField; i++ {
		field := t.Field(i)
		name := field.Name
		if name == "ID" {
			cMap["id"] = i
			num++
			if len(cMap) < num {
				return columns, errors.New("字段:: id  error")
			}
			continue
		}

		// 过滤掉首字母小写的字段
		if unicode.IsLower([]rune(name)[0]) {
			continue
		}
		name = utils.Camel2Case(name)

		if tag := field.Tag.Get("core"); tag == "-" {
			continue
		}

		if tag := field.Tag.Get("ldb"); tag != "" {
			name = tag
			cMap[name] = i
			num++
			if len(cMap) < num {
				return columns, errors.New("字段::" + "error")
			}
			continue
		}

		fieldNamePrefix := c.FieldNamePrefix
		if fieldNamePrefix != "" {
			cMap[fieldNamePrefix+name] = i
			num++
			if len(cMap) < num {
				return columns, errors.New("字段::" + "error")
			}
			continue
		}

		cMap[name] = i
		num++
		if len(cMap) < num {
			return columns, errors.New("字段::" + "error")
		}
	}
	arr := make([]string, len(cMap))

	var i = 0
	for s := range cMap {
		arr[i] = s
		i++
	}
	return arr, nil
}

//	 可以缓存
//
//		主键Id、ID，都转化为id
//
// tag== ldb:name  可以自定义名字
// tag== core:-  跳过
// 过滤掉首字母小写的字段
// 获取model对应的数据字段名：和其在model中的index下标
func (c OrmConf) getStructMappingColumns(t reflect.Type) (map[string]int, error) {
	cMap := make(map[string]int)

	numField := t.NumField()
	var num = 0
	for i := 0; i < numField; i++ {
		field := t.Field(i)
		name := field.Name

		if name == "ID" {
			cMap["id"] = i
			num++
			if len(cMap) < num {
				return cMap, errors.New("字段::id" + "error")
			}
			continue
		}

		// 过滤掉首字母小写的字段
		if unicode.IsLower([]rune(name)[0]) {
			continue
		}
		name = utils.Camel2Case(name)

		if tag := field.Tag.Get("lorm"); tag == "-" {
			continue
		}

		if tag := field.Tag.Get("lorm"); tag != "" {
			name = tag
			cMap[name] = i
			num++
			if len(cMap) < num {
				return cMap, errors.New("字段::" + "error")
			}
			continue
		}

		fieldNamePrefix := c.FieldNamePrefix
		if fieldNamePrefix != "" {
			cMap[fieldNamePrefix+name] = i
			num++
			if len(cMap) < num {
				return cMap, errors.New("字段::" + "error")
			}
			continue
		}

		cMap[name] = i
		num++
		if len(cMap) < num {
			return cMap, errors.New("字段::" + "error")
		}
	}

	return cMap, nil
}

type compCV struct {
	//有效字段列表
	columns []string
	//有效值列表
	columnValues []field.Value

	//零值字段列表
	modelZeroFieldNames []string

	//所有字段列表
	modelAllFieldNames []string
}

// 获取 struct 对应的字段名 和 其值
func (c OrmConf) getStructCV(v reflect.Value) (compCV, error) {
	t := v.Type()
	cv := compCV{
		columns:             make([]string, 0),
		columnValues:        make([]field.Value, 0),
		modelZeroFieldNames: make([]string, 0),
		modelAllFieldNames:  make([]string, 0),
	}
	mappingColumns, err := c.getStructMappingColumns(t)
	if err != nil {
		return cv, err
	}

	for column, i := range mappingColumns {
		fieldV := v.Field(i)
		if utils.IsSoftDelFieldType(fieldV.Type()) {
			continue
		}
		inter := getFieldInterZero(fieldV)
		cv.modelAllFieldNames = append(cv.modelAllFieldNames, column)
		if inter != nil {
			cv.columns = append(cv.columns, column)
			cv.columnValues = append(cv.columnValues, field.Value{
				Type:  field.Val,
				Value: inter,
			})
		} else {
			cv.modelZeroFieldNames = append(cv.modelZeroFieldNames, column)
		}
	}

	return cv, nil
}

// 获取map[string]any 对应的字段名 和 其值
func getMapCV(v reflect.Value) (compCV, error) {
	cv := compCV{
		columns:             make([]string, 0),
		columnValues:        make([]field.Value, 0),
		modelZeroFieldNames: make([]string, 0),
		modelAllFieldNames:  make([]string, 0),
	}

	for _, k := range v.MapKeys() {
		inter := getFieldInter(v.MapIndex(k))

		cv.columns = append(cv.columns, k.String())
		cv.columnValues = append(cv.columnValues, inter)
	}
	return cv, nil
}

// 获取comp :struct 对应的字段名
func (c OrmConf) getStructC(t reflect.Type) (compCV, error) {
	cv := compCV{
		columns:             make([]string, 0),
		columnValues:        make([]field.Value, 0),
		modelZeroFieldNames: make([]string, 0),
		modelAllFieldNames:  make([]string, 0),
	}
	mappingColumns, err := c.getStructMappingColumns(t)
	if err != nil {
		return cv, err
	}
	for column := range mappingColumns {
		cv.modelAllFieldNames = append(cv.modelAllFieldNames, column)
	}
	return cv, nil
}

// 获取 rows 返回数据，每个字段 对应 struct 的字段 下标
func (c OrmConf) getColFieldIndexLinkMap(columns []string, t reflect.Type) (ColFieldIndexLinkMap, error) {
	if isValuerType(t) {
		return ColFieldIndexLinkMap{}, nil
	}

	colNum := len(columns)
	cfm := make([]int, colNum)
	fm, err := getFieldMap(t, c.FieldNamePrefix)
	if err != nil {
		return nil, err
	}

	validNum := 0
	for i, column := range columns {
		index, ok := fm[column]
		if !ok {
			cfm[i] = -1
			continue
		}
		cfm[i] = index
		validNum++
	}

	if colNum == 1 && validNum == 0 {
		return ColFieldIndexLinkMap{}, nil
	}
	return cfm, nil
}

// tableName表名
// keys
// hasTen true开启多租户
func (c OrmConf) genDelSqlCommon(tableName string, keys []string) []byte {
	var bb bytes.Buffer

	//hasTen := c.TenantIdFieldName != "" && !c.TenantIgnoreTableFun(tableName)
	//whereSql := c.GenWhere(keys, hasTen)

	//logicDeleteSetSql := c.LogicDeleteSetSql
	//logicDeleteYesSql := c.LogicDeleteYesSql
	//if logicDeleteSetSql == "" {
	//	bb.WriteString("DELETE FROM ")
	//	bb.WriteString(tableName)
	//	bb.WriteString(string(whereSql))
	//} else {
	//	bb.WriteString("UPDATE ")
	//	bb.WriteString(tableName)
	//	bb.WriteString(" SET ")
	//	bb.WriteString(logicDeleteSetSql)
	//	bb.WriteString(string(whereSql))
	//	bb.WriteString(" and ")
	//	bb.WriteString(logicDeleteYesSql)
	//}
	return bb.Bytes()
}

// tableName表名
// keys
// hasTen true开启多租户
func (c OrmConf) genDelSqlByWhere(tableName string, where []byte) []byte {
	//hasTen := c.TenantIdFieldName != "" && !c.TenantIgnoreTableFun(tableName)

	var bb bytes.Buffer
	//whereSql := c.whereExtra(where, hasTen)
	//
	//logicDeleteSetSql := c.LogicDeleteSetSql
	//logicDeleteYesSql := c.LogicDeleteYesSql
	//lgSql := strings.ReplaceAll(logicDeleteSetSql, "lg.", "")
	//logicDeleteYesSql = strings.ReplaceAll(logicDeleteYesSql, "lg.", "")
	//if logicDeleteSetSql == lgSql {
	//	bb.WriteString("DELETE FROM ")
	//	bb.WriteString(tableName)
	//	bb.Write(whereSql)
	//} else {
	//	bb.WriteString("UPDATE ")
	//	bb.WriteString(tableName)
	//	bb.WriteString(" SET ")
	//	bb.WriteString(lgSql)
	//	bb.Write(whereSql)
	//	bb.WriteString(" and ")
	//	bb.WriteString(logicDeleteYesSql)
	//}
	return bb.Bytes()
}

// GenWhere 有tenantId功能
func (c OrmConf) GenWhere(keys []string, hasTen bool) []byte {
	if hasTen {
		keys = append(keys, c.TenantIdFieldName)
	}
	if len(keys) == 0 {
		return []byte("")
	}

	var bb bytes.Buffer
	bb.WriteString(" WHERE ")
	bb.WriteString(keys[0])
	bb.WriteString(" = ? ")
	for i := 1; i < len(keys); i++ {
		bb.WriteString(" AND ")
		bb.WriteString(keys[i])
		bb.WriteString(" = ? ")
	}

	return bb.Bytes()
}

// 有tenantid功能
func (c OrmConf) whereExtra(where []byte, hasTen bool) []byte {
	var bb bytes.Buffer
	//bb.Write(where)
	//
	//logicDeleteYesSql := c.LogicDeleteYesSql
	//lg := strings.ReplaceAll(logicDeleteYesSql, "lg.", "")
	//if lg != logicDeleteYesSql {
	//	bb.WriteString(" and ")
	//	bb.WriteString(lg)
	//}
	//
	//if hasTen {
	//	bb.WriteString(" AND ")
	//	bb.WriteString(c.TenantIdFieldName)
	//	bb.WriteString(" = ? ")
	//}

	return bb.Bytes()
}

// tableName表名
// columns
func (c OrmConf) genSelectSqlCommon(tableName string, columns []string) []byte {

	var bb bytes.Buffer
	bb.WriteString(" SELECT ")
	for i, column := range columns {
		if i == 0 {
			bb.WriteString(column)
		} else {
			bb.WriteString(" , ")
			bb.WriteString(column)
		}
	}
	bb.WriteString(" FROM ")
	bb.WriteString(tableName)
	return bb.Bytes()
}
