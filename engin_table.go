package lorm

import (
	"bytes"
	"database/sql"
	"github.com/lontten/lorm/utils"
	"github.com/pkg/errors"
	"reflect"
	"strings"
)

type EngineTable struct {
	dialect Dialect

	ctx OrmContext
}

//v0.6
func (e EngineTable) queryLn(query string, args ...interface{}) (int64, error) {
	rows, err := e.dialect.query(query, args...)
	if err != nil {
		return 0, err
	}
	return ormConfig.ScanLn(rows, e.ctx.dest)
}

//v0.6
func (e EngineTable) queryLnBatch(query string, args [][]interface{}) (int64, error) {
	stmt, err := e.dialect.queryBatch(query)
	if err != nil {
		return 0, err
	}

	rowss := make([]*sql.Rows, 0)
	for _, arg := range args {
		rows, err := stmt.Query(arg...)
		if err != nil {
			return 0, err
		}
		rowss = append(rowss, rows)
	}

	return ormConfig.ScanLnBatch(rowss, utils.ToSlice(e.ctx.destValue))
}

//v0.6
func (e EngineTable) query(query string, args ...interface{}) (int64, error) {
	rows, err := e.dialect.query(query, args...)
	if err != nil {
		return 0, err
	}
	return ormConfig.Scan(rows, e.ctx.dest)
}

//v0.6
func (e EngineTable) queryBatch(query string, args [][]interface{}) (int64, error) {
	stmt, err := e.dialect.queryBatch(query)
	if err != nil {
		return 0, err
	}

	rowss := make([]*sql.Rows, 0)
	for _, arg := range args {
		rows, err := stmt.Query(arg...)
		if err != nil {
			return 0, err
		}
		rowss = append(rowss, rows)
	}

	return ormConfig.ScanBatch(rowss, utils.ToSlice(e.ctx.destValue))
}

//v0.6
// *.comp / slice.comp
//target dest 一个comp-struct，或者一个slice-comp-struct
func (e *EngineTable) setTargetDestSlice(v interface{}) {
	if e.ctx.err != nil {
		return
	}
	e.ctx.initTargetDestSlice(v)
	e.ctx.checkTargetDestField()
	e.initTableName()
}

//v0.6
//*.comp
//target dest 一个comp-struct
func (e *EngineTable) setTargetDest(v interface{}) {
	if e.ctx.err != nil {
		return
	}
	e.ctx.initTargetDest(v)
	e.ctx.checkTargetDestField()
	e.initTableName()
}

//v0.6
func (e *EngineTable) setTargetDestOnlyTableName(v interface{}) {
	if e.ctx.err != nil {
		return
	}
	e.ctx.initTargetDestOnlyBaseValue(v)
	e.initTableName()
}

type OrmTableCreate struct {
	base EngineTable
}

type OrmTableSelect struct {
	base EngineTable

	query string
	args  []interface{}
}

type OrmTableSelectWhere struct {
	base EngineTable
}

type OrmTableUpdate struct {
	base EngineTable
}

type OrmTableDelete struct {
	base EngineTable
}

// Create
//v0.6
//1.ptr
//2.comp-struct
//3.slice-comp-struct
func (e EngineTable) Create(v interface{}) (num int64, err error) {
	e.setTargetDestSlice(v)
	e.initColumnsValue()
	if e.ctx.err != nil {
		return 0, e.ctx.err
	}
	sqlStr := e.ctx.tableCreateGen()

	if e.ctx.isSlice {
		return e.dialect.execBatch(sqlStr, e.ctx.columnValues)
	}
	return e.dialect.exec(sqlStr, e.ctx.columnValues[0])
}

// CreateOrUpdate
//v0.6
//只能一个
//1.ptr
//2.comp-struct
func (e EngineTable) CreateOrUpdate(v interface{}) OrmTableCreate {
	e.setTargetDest(v)
	e.initColumnsValue()
	return OrmTableCreate{base: e}
}

// ByPrimaryKey
//v0.6
//ptr
//single / comp复合主键
func (orm OrmTableCreate) ByPrimaryKey(v interface{}) (int64, error) {
	if v == nil {
		return 0, errors.New("ByPrimaryKey is nil")
	}
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}

	keyNum := len(ctx.primaryKeyNames)

	cs := ctx.columns
	cvs := ctx.columnValues[0]
	tableName := ctx.tableName

	idValues := make([]interface{}, 0)

	columns, values, err := getCompCV(v)
	if err != nil {
		return 0, err
	}
	//只要主键字段
	for _, key := range ctx.primaryKeyNames {
		for i, c := range columns {
			if c == key {
				idValues = append(idValues, values[i])
				continue
			}
		}
	}

	idLen := len(idValues)
	if idLen == 0 {
		return 0, errors.New("no pk")
	}
	if keyNum != idLen {
		return 0, errors.New("comp pk num err")
	}

	whereStr := ctx.genWhereByPrimaryKey()

	var bb bytes.Buffer
	bb.WriteString("SELECT 1 ")
	bb.WriteString(" FROM ")
	bb.WriteString(tableName)
	bb.Write(whereStr)
	bb.WriteString("limit 1")
	rows, err := base.dialect.query(bb.String(), idValues)
	if err != nil {
		return 0, err
	}
	//update
	if rows.Next() {
		bb.Reset()
		bb.WriteString("UPDATE ")
		bb.WriteString(tableName)
		bb.WriteString(" SET ")
		bb.WriteString(ctx.tableUpdateArgs2SqlStr(cs))
		bb.Write(whereStr)
		cvs = append(cvs, idValues)

		return base.dialect.exec(bb.String(), cvs...)
	}

	columnSqlStr := ctx.tableCreateArgs2SqlStr()

	bb.Reset()
	bb.WriteString("INSERT INTO ")
	bb.WriteString(tableName)
	bb.WriteString(columnSqlStr)

	return base.dialect.exec(bb.String(), cvs...)
}

//v0.6
//ptr-comp
func (orm OrmTableCreate) ByModel(v interface{}) (int64, error) {
	if v == nil {
		return 0, errors.New("ByModel is nil")
	}
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}

	c := ctx.columns
	cv := ctx.columnValues[0]
	tableName := ctx.tableName

	columns, values, err := getCompCV(v)
	if err != nil {
		return 0, err
	}
	where := ctx.genWhere(columns)

	var bb bytes.Buffer
	bb.WriteString("SELECT 1 ")
	bb.WriteString(" FROM ")
	bb.WriteString(tableName)
	bb.Write(where)
	bb.WriteString("limit 1")
	rows, err := base.dialect.query(bb.String(), values...)
	if err != nil {
		return 0, err
	}
	//update
	if rows.Next() {
		bb.Reset()
		bb.WriteString("UPDATE ")
		bb.WriteString(tableName)
		bb.WriteString(" SET ")
		bb.WriteString(ctx.tableUpdateArgs2SqlStr(c))
		bb.Write(where)
		cv = append(cv, values...)

		return base.dialect.exec(bb.String(), cv...)
	}
	columnSqlStr := ctx.tableCreateArgs2SqlStr()

	bb.Reset()
	bb.WriteString("INSERT INTO ")
	bb.WriteString(tableName)
	bb.WriteString(columnSqlStr)

	return base.dialect.exec(bb.String(), cv...)
}

func (orm OrmTableCreate) ByWhere(w *WhereBuilder) (int64, error) {
	if w == nil {
		return 0, errors.New("ByWhere is nil")
	}
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}
	c := ctx.columns
	cv := ctx.columnValues[0]
	tableName := ctx.tableName


	wheres := w.context.wheres
	args := w.context.args

	var bb bytes.Buffer
	bb.WriteString("WHERE ")
	for i, where := range wheres {
		if i == 0 {
			bb.WriteString(" WHERE " + where)
			continue
		}
		bb.WriteString(" AND " + where)
	}
	whereSql := bb.String()

	bb.Reset()
	bb.WriteString("SELECT 1 ")
	bb.WriteString(" FROM ")
	bb.WriteString(tableName)
	bb.WriteString(whereSql)
	bb.WriteString("limit 1")

	rows, err := base.dialect.query(bb.String(), args...)
	if err != nil {
		return 0, err
	}
	//update
	if rows.Next() {
		bb.Reset()
		bb.WriteString("UPDATE ")
		bb.WriteString(tableName)
		bb.WriteString(" SET ")
		bb.WriteString(ctx.tableUpdateArgs2SqlStr(c))
		bb.WriteString(whereSql)
		cv = append(cv, args)

		return base.dialect.exec(bb.String(), cv...)
	}
	columnSqlStr := ctx.tableCreateArgs2SqlStr()

	bb.Reset()
	bb.WriteString("INSERT INTO ")
	bb.WriteString(tableName)
	bb.WriteString(columnSqlStr)

	return base.dialect.exec(bb.String(), cv...)
}

//delete
func (e EngineTable) Delete(v interface{}) OrmTableDelete {
	e.setTargetDestOnlyTableName(v)
	return OrmTableDelete{base: e}
}

//v0.6
//[]
//single -> 单主键
//comp -> 复合主键
func (orm OrmTableDelete) ByPrimaryKey(v ...interface{}) (int64, error) {
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}

	idLen := len(v)
	if idLen == 0 {
		return 0, errors.New("ByPrimaryKey arg len num 0")
	}

	idNames := ctx.primaryKeyNames
	pkLen := len(idNames)
	if pkLen == 1 { //单主键
		for _, i := range v {
			value := reflect.ValueOf(i)
			_, value = basePtrDeepValue(value)
			ctyp := checkCompTypeValue(value, false)
			if ctyp != Single {
				return 0, errors.New("ByPrimaryKey typ err")
			}
		}
	} else {
		for _, i := range v {
			value := reflect.ValueOf(i)
			_, value = basePtrDeepValue(value)
			ctyp := checkCompTypeValue(value, false)
			if ctyp != Composite {
				return 0, errors.New("ByPrimaryKey typ err")
			}

			cv, _, err := getCompValueCV(value)
			if err != nil {
				return 0, err
			}
			if len(cv) != pkLen {
				return 0, errors.New("复合主键，filed数量 len err")
			}

		}
	}

	base.initPrimaryKeyName()
	ctx.checkValidPrimaryKey(v)

	delSql := ctx.genDelByPrimaryKey()
	return base.dialect.exec(string(delSql), v...)
}

// ByModel
//v0.6
//ptr
//comp,只能一个comp-struct
func (orm OrmTableDelete) ByModel(v interface{}) (int64, error) {
	if v == nil {
		return 0, errors.New("ByModel is nil")
	}
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}

	columns, values, err := getCompCV(v)
	if err != nil {
		return 0, err
	}

	delSql := ctx.genDel(columns)
	return base.dialect.exec(string(delSql), values...)
}

//v0.6
//排除 nil 字段
func getCompCV(v interface{}) ([]string, []interface{}, error) {
	value := reflect.ValueOf(v)
	_, value = basePtrDeepValue(value)
	ctyp := checkCompTypeValue(value, false)
	if ctyp != Composite {
		return nil, nil, errors.New("ByModel typ err")
	}
	err := checkCompValidFieldNuller(value)
	if err != nil {
		return nil, nil, err
	}

	columns, values, err := ormConfig.getCompColumnsValueNoNil(value)
	if err != nil {
		return nil, nil, err
	}
	if len(columns) < 1 {
		return nil, nil, errors.New("where model valid field need ")
	}
	return columns, values, nil
}
//v0.6
//排除 nil 字段
func getCompValueCV(v reflect.Value) ([]string, []interface{}, error) {
	ctyp := checkCompTypeValue(v, false)
	if ctyp != Composite {
		return nil, nil, errors.New("ByModel typ err")
	}
	err := checkCompValidFieldNuller(v)
	if err != nil {
		return nil, nil, err
	}

	columns, values, err := ormConfig.getCompColumnsValueNoNil(v)
	if err != nil {
		return nil, nil, err
	}
	if len(columns) < 1 {
		return nil, nil, errors.New("where model valid field need ")
	}
	return columns, values, nil
}

// ByWhere
//v0.6
func (orm OrmTableDelete) ByWhere(w *WhereBuilder) (int64, error) {
	if w == nil {
		return 0, nil
	}
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}

	wheres := w.context.wheres
	args := w.context.args

	delSql := ctx.genDel(wheres)
	return base.dialect.exec(string(delSql), args...)
}

// Update
//v0.6
func (e EngineTable) Update(v interface{}) OrmTableUpdate {
	e.setTargetDestSlice(v)
	e.initColumnsValue()
	return OrmTableUpdate{base: e}
}

func (orm OrmTableUpdate) ByPrimaryKey(v ...interface{}) (int64, error) {
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}

	err := checkStructValidFieldNuller(reflect.ValueOf(v))
	if err != nil {
		return 0, err
	}

	base.initPrimaryKeyName()

	tableName := ctx.tableName
	c := ctx.columns
	cv := ctx.columnValues

	var sb strings.Builder
	sb.WriteString(" UPDATE ")
	sb.WriteString(tableName)
	sb.WriteString(" SET ")
	sb.WriteString(ctx.tableUpdateArgs2SqlStr(c))
	sb.WriteString(" WHERE ")
	//sb.WriteString(orm.base.primaryKeyNames)
	sb.WriteString(" = ? ")
	cv = append(cv, v)
	return base.dialect.exec(sb.String(), cv)
}

func (orm OrmTableUpdate) ByModel(v interface{}) (int64, error) {
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}
	columns, values, err := getCompCV(v)
	if err != nil {
		return 0, err
	}

	tableName := ctx.tableName
	c := ctx.columns
	cv := ctx.columnValues[0]

	var sb strings.Builder
	sb.WriteString(" UPDATE ")
	sb.WriteString(tableName)
	sb.WriteString(" SET ")
	sb.WriteString(ctx.tableUpdateArgs2SqlStr(c))

	where := genWhere(columns)
	sb.WriteString(" WHERE ")
	sb.WriteString(string(where))

	cv = append(cv, values...)

	return base.dialect.exec(sb.String(), cv...)
}

func (orm OrmTableUpdate) ByWhere(w *WhereBuilder) (int64, error) {
	base := orm.base
	if err := base.ctx.err; err != nil {
		return 0, err
	}

	if w == nil {
		return 0, nil
	}
	wheres := w.context.wheres
	args := w.context.args

	tableName := base.ctx.tableName
	c := base.ctx.columns
	cv := base.ctx.columnValues[0]

	var sb strings.Builder
	sb.WriteString(" UPDATE ")
	sb.WriteString(tableName)
	sb.WriteString(" SET ")
	sb.WriteString(base.ctx.tableUpdateArgs2SqlStr(c))
	sb.WriteString(" WHERE ")
	for i, where := range wheres {
		if i == 0 {
			sb.WriteString(where)
			continue
		}
		sb.WriteString(" AND " + where)
	}

	cv = append(cv, args...)

	return base.dialect.exec(sb.String(), cv...)
}

//select
func (e EngineTable) Select(v interface{}) OrmTableSelect {
	e.setTargetDestOnlyTableName(v)
	if e.ctx.err != nil {
		return OrmTableSelect{base: e}
	}

	return OrmTableSelect{base: e}
}

//func (orm OrmTableSelect) ByPrimaryKey(v ...interface{}) (int64, error) {
//	if err := orm.base.ctx.err; err != nil {
//		return 0, err
//	}
//
//	err := checkStructValidFieldNuller(reflect.ValueOf(v))
//	if err != nil {
//		return 0, err
//	}
//	err = orm.base.initColumns()
//	if err != nil {
//		return 0, err
//	}
//	orm.base.initPrimaryKeyName()
//	tableName := orm.base.ctx.tableName
//	c := orm.base.ctx.columns
//
//	var sb strings.Builder
//	sb.WriteString(" SELECT ")
//	for i, column := range c {
//		if i == 0 {
//			sb.WriteString(column)
//		} else {
//			sb.WriteString(" , ")
//			sb.WriteString(column)
//		}
//	}
//	sb.WriteString(" FROM ")
//	sb.WriteString(tableName)
//	sb.WriteString(" WHERE ")
//	//sb.WriteString(orm.base.primaryKeyNames)
//	sb.WriteString(" = ? ")
//
//	return orm.base.queryLn(sb.String(), v)
//}

func (orm OrmTableSelectWhere) getOne() (int64, error) {
	if err := orm.base.ctx.err; err != nil {
		return 0, err
	}

	tableName := orm.base.ctx.tableName
	c := orm.base.ctx.columns

	var sb strings.Builder
	sb.WriteString(" SELECT ")
	for i, column := range c {
		if i == 0 {
			sb.WriteString(column)
		} else {
			sb.WriteString(" , ")
			sb.WriteString(column)
		}
	}
	sb.WriteString(" FROM ")
	sb.WriteString(tableName)
	sb.WriteString(" WHERE ")
	//sb.WriteString(orm.base.primaryKeyNames)
	sb.WriteString(" = ? ")

	return orm.base.queryLn(sb.String(), orm.base.ctx.dest)
}

func (orm OrmTableSelectWhere) getList() (int64, error) {
	if err := orm.base.ctx.err; err != nil {
		return 0, err
	}

	tableName := orm.base.ctx.tableName
	c := orm.base.ctx.columns

	var sb strings.Builder
	sb.WriteString("SELECT ")
	for i, column := range c {
		if i == 0 {
			sb.WriteString(column)
		} else {
			sb.WriteString(" , ")
			sb.WriteString(column)
		}
	}
	sb.WriteString(" FROM ")
	sb.WriteString(tableName)
	sb.WriteString("WHERE ")
	//sb.WriteString(orm.base.primaryKeyNames)
	sb.WriteString(" = ? ")

	return orm.base.queryLn(sb.String(), orm.base.ctx.dest)
}

//v0.6
//ptr-comp
func (orm OrmTableSelect) ByModel(v interface{}) (int64, error) {
	base := orm.base
	ctx := base.ctx
	if err := ctx.err; err != nil {
		return 0, err
	}

	columns, values, err := getCompCV(v)
	if err != nil {
		return 0, err
	}

	tableName := ctx.tableName
	c := ctx.columns

	var sb strings.Builder
	sb.WriteString("SELECT ")
	for i, column := range c {
		if i == 0 {
			sb.WriteString(column)
		} else {
			sb.WriteString(" , ")
			sb.WriteString(column)
		}
	}
	sb.WriteString(" FROM ")
	sb.WriteString(tableName)
	sb.WriteString(ctx.tableWhereArgs2SqlStr(columns))

	return base.queryLn(sb.String(), values...)
}

func (orm OrmTableSelect) ByWhere(w *WhereBuilder) (int64, error) {
	if err := orm.base.ctx.err; err != nil {
		return 0, err
	}

	if w == nil {
		return 0, errors.New("table select where can't nil")
	}
	orm.base.initColumns()
	orm.base.initPrimaryKeyName()

	wheres := w.context.wheres
	args := w.context.args

	tableName := orm.base.ctx.tableName
	c := orm.base.ctx.columns

	var sb strings.Builder
	sb.WriteString("SELECT ")
	for i, column := range c {
		if i == 0 {
			sb.WriteString(column)
		} else {
			sb.WriteString(" , ")
			sb.WriteString(column)
		}
	}
	sb.WriteString(" FROM ")
	sb.WriteString(tableName)
	sb.WriteString(" WHERE ")
	for i, where := range wheres {
		if i == 0 {
			sb.WriteString(where)
			continue
		}
		sb.WriteString(" AND " + where)
	}

	return orm.base.queryLn(sb.String(), args...)
}

//0.6
//初始化主键
func (e *EngineTable) initPrimaryKeyName() {
	if e.ctx.err != nil {
		return
	}
	e.ctx.primaryKeyNames = ormConfig.primaryKeys(e.ctx.tableName, e.ctx.destBaseValue)
}

//0.6
//初始化 表名
func (e *EngineTable) initTableName() {
	if e.ctx.err != nil {
		return
	}
	tableName, err := ormConfig.tableName(e.ctx.destBaseValue)
	if err != nil {
		e.ctx.err = err
		return
	}
	e.ctx.tableName = tableName
}

//0.6
//获取struct对应的字段名 和 其值，
//slice为全部，一个为非nil字段。
func (e *EngineTable) initColumnsValue() {
	if e.ctx.err != nil {
		return
	}
	columns, valuess, err := ormConfig.initColumnsValue(e.ctx.destValueArr)
	if err != nil {
		e.ctx.err = err
		return
	}
	e.ctx.columns = columns
	e.ctx.columnValues = valuess
	return
}

//v0.6
//获取struct对应的字段名 有效部分
func (e *EngineTable) initColumns() {
	if e.ctx.err != nil {
		return
	}

	columns, err := ormConfig.initColumns(e.ctx.destBaseValue)
	if err != nil {
		e.ctx.err = err
		return
	}
	e.ctx.columns = columns
}
