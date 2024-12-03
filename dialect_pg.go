package lorm

import (
	"errors"
	"github.com/lontten/lorm/field"
	"github.com/lontten/lorm/insert_type"
	"github.com/lontten/lorm/return_type"
	"github.com/lontten/lorm/utils"
	"strconv"
	"strings"
	"time"
)

type PgDialect struct {
	ctx *ormContext
}

// ===----------------------------------------------------------------------===//
// 获取上下文
// ===----------------------------------------------------------------------===//
func (d *PgDialect) getCtx() *ormContext {
	return d.ctx
}
func (d *PgDialect) initContext() *ormContext {
	d.ctx = &ormContext{
		ormConf:    d.ctx.ormConf,
		query:      &strings.Builder{},
		insertType: insert_type.Err,
	}
	return d.ctx
}
func (d *PgDialect) hasErr() bool {
	return d.ctx.err != nil
}
func (d *PgDialect) getErr() error {
	return d.ctx.err
}

// ===----------------------------------------------------------------------===//
// sql 方言化
// ===----------------------------------------------------------------------===//

func (d *PgDialect) prepare(query string) string {
	query = toPgSql(query)
	return query
}
func (d *PgDialect) exec(query string, args ...any) (string, []any) {
	query = toPgSql(query)
	return query, args
}

func (d *PgDialect) query(query string, args ...any) (string, []any) {
	query = toPgSql(query)
	return query, args
}

func (d *PgDialect) queryBatch(query string) string {
	query = toPgSql(query)

	//return m.ldb.Prepare(query)
	return query
}

func (m *PgDialect) insertOrUpdateByPrimaryKey(table string, fields []string, columns []string, args ...any) (string, []any) {
	return m.insertOrUpdateByUnique(table, fields, columns, args...)
}

func (p *PgDialect) insertOrUpdateByUnique(table string, fields []string, columns []string, args ...any) (string, []any) {
	cs := make([]string, 0)
	vs := make([]any, 0)

	for i, column := range columns {
		if utils.Contains(fields, column) {
			continue
		}
		cs = append(cs, column)
		vs = append(vs, args[i])
	}

	var query = "INSERT INTO " + table + "(" + strings.Join(columns, ",") +
		") VALUES (" + strings.Repeat(" ? ,", len(args)-1) +
		" ? ) ON CONFLICT (" + strings.Join(fields, ",") + ") DO"
	if len(vs) == 0 {
		query += "NOTHING"
	} else {
		query += " UPDATE SET " + strings.Join(cs, "= ? , ") + "= ? "
	}
	args = append(args, vs...)
	query = toPgSql(query)
	p.ctx.log.Println(query, args)
	//exec, err := m.ldb.Exec(query, args...)
	//if err != nil {
	//	if errors.As(err, &ErrNoPkOrUnique) {
	//		return 0, errors.New("insertOrUpdateByUnique fields need to be unique or primary key:" + strings.Join(fields, ",") + err.Error())
	//	}
	//	return 0, err
	//}
	return query, args
}
func (d *PgDialect) getSql() string {
	s := d.ctx.query.String()
	return toPgSql(s)
}

// insert 生成
func (d *PgDialect) tableInsertGen() {
	ctx := d.ctx
	if ctx.hasErr() {
		return
	}
	extra := ctx.extra
	set := extra.set
	columns := ctx.columns
	values := ctx.columnValues
	var query = d.ctx.query

	query.WriteString("INSERT INTO ")

	query.WriteString(ctx.tableName + " ")

	query.WriteString(" ( ")
	for i, v := range columns {
		if i == 0 {
			query.WriteString(v)
		} else {
			query.WriteString(" , " + v)
		}
	}
	query.WriteString(" ) ")
	query.WriteString(" VALUES ")
	query.WriteString("( ")
	for i, v := range values {
		if i > 0 {
			query.WriteString(" , ")
		}
		switch v.Type {
		case field.None:
			break
		case field.Null:
			query.WriteString("NULL")
			break
		case field.Now:
			query.WriteString("NOW()")
			break
		case field.UnixSecond:
			query.WriteString(strconv.Itoa(time.Now().Second()))
			break
		case field.UnixMilli:
			query.WriteString(strconv.FormatInt(time.Now().UnixMilli(), 10))
			break
		case field.UnixNano:
			query.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
			break
		case field.Val:
			query.WriteString(" ? ")
			ctx.args = append(ctx.args, v.Value)
			break
		case field.Increment:
			query.WriteString(columns[i] + " + ? ")
			ctx.args = append(ctx.args, v.Value)
			break
		case field.Expression:
			query.WriteString(v.Value.(string))
			break
		case field.ID:
			if len(ctx.primaryKeyNames) > 0 {
				ctx.err = errors.New("软删除标记为主键id，需要单主键")
				return
			}
			query.WriteString(ctx.primaryKeyNames[0])
			break
		}
	}
	query.WriteString(" ) ")

	if ctx.insertType == insert_type.Ignore || ctx.insertType == insert_type.Update {
		query.WriteString("ON CONFLICT (")
		query.WriteString(strings.Join(extra.duplicateKeyNames, ","))
		query.WriteString(") DO")
	}

	switch ctx.insertType {
	case insert_type.Ignore:
		query.WriteString(" NOTHING")
		break
	case insert_type.Update:
		query.WriteString(" UPDATE SET ")

		// 当未设置更新字段时，默认为所有字段
		if len(set.columns) == 0 && len(set.fieldNames) == 0 {
			list := append(ctx.columns, extra.columns...)
			list = append(list, set.columns...)
			for _, name := range list {
				find := utils.Find(extra.duplicateKeyNames, name)
				if find < 0 {
					set.fieldNames = append(set.fieldNames, name)
				}
			}
		}
		for i, name := range set.fieldNames {
			if i < len(set.fieldNames)-1 {
				query.WriteString(name + " = EXCLUDED." + name + ",")
			} else {
				query.WriteString(name + " = EXCLUDED." + name)
			}
		}

		for i, column := range set.columns {
			if i < len(set.columns)-1 {
				query.WriteString(column + " = ? , ")
			} else {
				query.WriteString(column + " = ? ")
			}
			ctx.args = append(ctx.args, set.columnValues[i].Value)
		}
		break
	default:
		break
	}

	// INSERT IGNORE 无法和 RETURNING 共存，当 INSERT IGNORE 时，不返回
	if ctx.scanIsPtr {
		switch expr := ctx.returnType; expr {
		case return_type.None:
			ctx.sqlIsQuery = true
			break
		case return_type.PrimaryKey:
			query.WriteString(" RETURNING " + strings.Join(ctx.primaryKeyNames, ","))
		case return_type.ZeroField:
			query.WriteString(" RETURNING " + strings.Join(ctx.modelZeroFieldNames, ","))
		case return_type.AllField:
			query.WriteString(" RETURNING " + strings.Join(ctx.modelAllFieldNames, ","))
		}
	}
	query.WriteString(";")
}

func (p *PgDialect) execBatch(query string, args [][]any) (string, [][]any) {
	query = toPgSql(query)
	//var num int64 = 0
	//stmt, err := m.ldb.Prepare(query)
	//defer stmt.Close()
	//if err != nil {
	//	return 0, err
	//}
	//for _, arg := range args {
	//	exec, err := stmt.Exec(arg...)
	//
	//	m.log.Println(query, arg)
	//	if err != nil {
	//		return num, err
	//	}
	//	rowsAffected, err := exec.RowsAffected()
	//	if err != nil {
	//		return num, err
	//	}
	//	num += rowsAffected
	//}
	return query, args
}

// ===----------------------------------------------------------------------===//
// 工具
// ===----------------------------------------------------------------------===//
func (p *PgDialect) appendBaseToken(token baseToken) {
	p.ctx.baseTokens = append(p.ctx.baseTokens, token)
}
func toPgSql(sql string) string {
	var i = 1
	for {
		t := strings.Replace(sql, "?", " $"+strconv.Itoa(i)+" ", 1)
		if t == sql {
			break
		}
		i++
		sql = t
	}
	return sql
}

// ===----------------------------------------------------------------------===//
// 中间服务
// ===----------------------------------------------------------------------===//
func (p *PgDialect) toSqlInsert() (string, []any) {
	tableName := p.ctx.tableName
	return tableName, nil
}

func (p *PgDialect) parse(c Clause) (string, error) {
	sb := strings.Builder{}
	switch c.Type {
	case Eq:
		sb.WriteString(c.query + " = ?")
	case Neq:
		sb.WriteString(c.query + " <> ?")
	case Less:
		sb.WriteString(c.query + " < ?")
	case LessEq:
		sb.WriteString(c.query + " <= ?")
	case Greater:
		sb.WriteString(c.query + " > ?")
	case GreaterEq:
		sb.WriteString(c.query + " >= ?")
	case Like:
		sb.WriteString(c.query + " LIKE ?")
	case NotLike:
		sb.WriteString(c.query + " NOT LIKE ?")
	case In:
		sb.WriteString(c.query + " IN (")
		sb.WriteString(gen(c.argsNum))
		sb.WriteString(")")
	case NotIn:
		sb.WriteString(c.query + " NOT IN (")
		sb.WriteString(gen(c.argsNum))
		sb.WriteString(")")
	case Between:
		sb.WriteString(c.query + " BETWEEN ? AND ?")
	case NotBetween:
		sb.WriteString(c.query + " NOT BETWEEN ? AND ?")
	case IsNull:
		sb.WriteString(c.query + " IS NULL")
	case IsNotNull:
		sb.WriteString(c.query + " IS NOT NULL")
	case IsFalse:
		sb.WriteString(c.query + " IS FALSE")
	default:
		return "", errors.New("unknown where token type")
	}

	return sb.String(), nil
}
