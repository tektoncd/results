package main

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
	"log"
	"regexp"
	"strings"
)

var (
	regFullDataType = regexp.MustCompile(`\D*(\d+)\D?`)
)

type printSQLLogger struct {
	logger.Interface
}

type Migrator struct {
	migrator.Migrator
}

func NewMigrator(db *gorm.DB) *Migrator {
	return &Migrator{
		migrator.Migrator{
			Config: migrator.Config{
				DB:                          db,
				Dialector:                   db.Dialector,
				CreateIndexAfterCreateTable: true,
			},
		},
	}
}

// Migrate values to one defined in the models.
// This methode is a modified version of GORM's AutoMigrate method.
func (m *Migrator) Migrate(values ...interface{}) error {
	for _, value := range m.ReorderModels(values, true) {
		queryTx := m.DB.Session(&gorm.Session{})
		execTx := queryTx
		if m.DB.DryRun {
			queryTx.DryRun = false
			execTx = m.DB.Session(&gorm.Session{Logger: &printSQLLogger{Interface: m.DB.Logger}})
		}
		if !queryTx.Migrator().HasTable(value) {
			if err := execTx.Migrator().CreateTable(value); err != nil {
				return err
			}
		} else {
			if err := m.RunWithValue(value, func(stmt *gorm.Statement) (errr error) {
				columnTypes, err := queryTx.Migrator().ColumnTypes(value)
				if err != nil {
					return err
				}

				for _, dbName := range stmt.Schema.DBNames {
					field := stmt.Schema.FieldsByDBName[dbName]
					var foundColumn gorm.ColumnType

					for _, columnType := range columnTypes {
						if columnType.Name() == dbName {
							foundColumn = columnType
							break
						}
					}

					if foundColumn == nil {
						if err := execTx.Migrator().AddColumn(value, dbName); err != nil {
							return err
						}
					} else if err := m.MigrateColumn(value, field, foundColumn); err != nil {
						return err
					}
				}

				if !m.DB.DisableForeignKeyConstraintWhenMigrating && !m.DB.IgnoreRelationshipsWhenMigrating {
					for _, rel := range stmt.Schema.Relationships.Relations {
						if rel.Field.IgnoreMigration {
							continue
						}
						if constraint := rel.ParseConstraint(); constraint != nil &&
							constraint.Schema == stmt.Schema && !queryTx.Migrator().HasConstraint(value, constraint.Name) {
							if err := execTx.Migrator().CreateConstraint(value, constraint.Name); err != nil {
								return err
							}
						}
					}
				}

				for _, chk := range stmt.Schema.ParseCheckConstraints() {
					if !queryTx.Migrator().HasConstraint(value, chk.Name) {
						if err := execTx.Migrator().CreateConstraint(value, chk.Name); err != nil {
							return err
						}
					}
				}

				for _, idx := range stmt.Schema.ParseIndexes() {
					if !queryTx.Migrator().HasIndex(value, idx.Name) {
						if err := execTx.Migrator().CreateIndex(value, idx.Name); err != nil {
							return err
						}
					}
				}

				return nil
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Migrator) MigrateColumn(value interface{}, field *schema.Field, columnType gorm.ColumnType) error {

	fullDataType := strings.TrimSpace(strings.ToLower(m.DB.Migrator().FullDataTypeOf(field).SQL))
	realDataType := strings.ToLower(columnType.DatabaseTypeName())

	var (
		alterColumn, isSameType bool
	)

	// check type
	if !strings.HasPrefix(fullDataType, realDataType) {
		// check type aliases
		aliases := m.DB.Migrator().GetTypeAliases(realDataType)
		for _, alias := range aliases {
			if strings.HasPrefix(fullDataType, alias) {
				isSameType = true
				break
			}
		}

		if !isSameType {
			alterColumn = true
		}
	}

	if !isSameType {
		// check size
		if length, ok := columnType.Length(); length != int64(field.Size) {
			if length > 0 && field.Size > 0 {
				alterColumn = true
			} else {
				// has size in data type and not equal
				matches := regFullDataType.FindAllStringSubmatch(fullDataType, -1)
				if !field.PrimaryKey &&
					(len(matches) == 1 && matches[0][1] != fmt.Sprint(length) && ok) {
					alterColumn = true
				}
			}
		}

		// check precision
		if precision, _, ok := columnType.DecimalSize(); ok && int64(field.Precision) != precision {
			if regexp.MustCompile(fmt.Sprintf("[^0-9]%d[^0-9]", field.Precision)).MatchString(m.DataTypeOf(field)) {
				alterColumn = true
			}
		}
	}

	if alterColumn && !field.IgnoreMigration {
		return m.AlterColumn(value, field.DBName)
	} else {
		columnTypeString, _ := columnType.ColumnType()
		log.Printf("SKIP: Table: %s, Column: %s, OldType: %s, NewType: %s\n", field.Schema.Table, field.DBName, columnTypeString, fullDataType)
	}

	return nil
}

// AlterColumn alter value's `field` column's type based on schema definition
func (m *Migrator) AlterColumn(value interface{}, field string) error {
	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if field := stmt.Schema.LookUpField(field); field != nil {
			var (
				columnTypes, _  = m.DB.Migrator().ColumnTypes(value)
				fieldColumnType *migrator.ColumnType
			)
			for _, columnType := range columnTypes {
				if columnType.Name() == field.DBName {
					fieldColumnType, _ = columnType.(*migrator.ColumnType)
				}
			}

			fieldType := clause.Expr{SQL: m.DataTypeOf(field)}
			// check for typeName and SQL name
			isSameType := true
			if fieldColumnType.DatabaseTypeName() != fieldType.SQL {
				isSameType = false
				// if different, also check for aliases
				aliases := m.GetTypeAliases(fieldColumnType.DatabaseTypeName())
				for _, alias := range aliases {
					if strings.HasPrefix(fieldType.SQL, alias) {
						isSameType = true
						break
					}
				}
			}

			// not same, migrate
			if !isSameType {
				log.Printf("ALTER: Table: %s, Column: %s, OldType: %s, NewType: %s\n", stmt.Table, field.DBName, fieldColumnType.DatabaseTypeName(), fieldType.SQL)

				// for 'bytea' fields, use the postgres convert_from() function to format the data.
				if fieldColumnType.DatabaseTypeName() == "bytea" {
					if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? TYPE ? USING convert_from(?, 'UTF-8')::?",
						m.CurrentTable(stmt), clause.Column{Name: field.DBName}, fieldType, clause.Column{Name: field.DBName}, fieldType).Error; err != nil {
						return err
					}
				} else {
					if err := m.DB.Exec("ALTER TABLE ? ALTER COLUMN ? TYPE ? USING ?::?",
						m.CurrentTable(stmt), clause.Column{Name: field.DBName}, fieldType, clause.Column{Name: field.DBName}, fieldType).Error; err != nil {
						return err
					}
				}
			} else {
				log.Printf("SKIP: Table: %s, Column: %s, OldType: %s, NewType: %s\n", stmt.Table, field.DBName, fieldColumnType.DatabaseTypeName(), fieldType.SQL)
			}
			return nil
		}
		return fmt.Errorf("failed to look up field with name: %s", field)
	})

	if err != nil {
		return err
	}
	m.resetPreparedStmts()
	return nil
}

// should reset prepared stmts when table changed
func (m *Migrator) resetPreparedStmts() {
	if m.DB.PrepareStmt {
		if pdb, ok := m.DB.ConnPool.(*gorm.PreparedStmtDB); ok {
			pdb.Reset()
		}
	}
}
