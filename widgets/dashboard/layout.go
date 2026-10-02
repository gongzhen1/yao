package dashboard

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/yaoapp/yao/widgets/component"
	"github.com/yaoapp/yao/widgets/mapping"
)

// Xgen trans to Xgen setting
func (layout *LayoutDSL) Xgen(data map[string]interface{}, excludes map[string]bool, mapping *mapping.Mapping) (*LayoutDSL, error) {
	clone, err := layout.Clone()
	if err != nil {
		return nil, err
	}

	// Filter
	if clone.Filter != nil {
		if clone.Filter.Actions != nil {
			clone.Filter.Actions = clone.Filter.Actions.Filter(excludes)
		}

		if clone.Filter.Columns != nil {
			columns := []component.InstanceDSL{}
			for _, column := range clone.Filter.Columns {
				id, has := mapping.Filters[column.Name]
				if !has {
					continue
				}

				if _, has := excludes[id]; has {
					continue
				}

				columns = append(columns, column)
			}
			clone.Filter.Columns = columns
		}
	}

	// Actions
	if clone.Actions != nil {
		clone.Actions = clone.Actions.Filter(excludes)
	}

	// Columns
	if clone.Dashboard != nil && clone.Dashboard.Columns != nil {
		columns := []component.InstanceDSL{}
		for _, column := range clone.Dashboard.Columns {

			// Container: keep all its own meta (name/title/x/y/width/height),
			// only filter its children. The container itself has no field definition.
			//
			// 注意不能只用 Rows != nil 判定：Clone() 是 JSON 往返，空数组 rows: []
			// 带 omitempty 会被丢掉，反序列化后 Rows 退化成 nil；
			// 因此把绝对定位的几何信息(X/Y)也作为容器信号。
			if column.Rows != nil || column.X != nil || column.Y != nil {
				new := column
				new.Rows = []component.InstanceDSL{}

				for _, row := range column.Rows {
					id, has := mapping.Columns[row.Name]
					if !has {
						continue
					}

					if _, has := excludes[id]; has {
						continue
					}
					new.Rows = append(new.Rows, row)
				}

				// Keep empty containers in absolute layout, they are pure layout nodes.
				if len(new.Rows) > 0 || new.X != nil {
					columns = append(columns, new)
				}
				continue
			}

			id, has := mapping.Columns[column.Name]
			if !has {
				continue
			}

			if _, has := excludes[id]; has {
				continue
			}

			columns = append(columns, column)
		}
		clone.Dashboard.Columns = columns
	}

	return clone, nil
}

// Clone layout for output
func (layout *LayoutDSL) Clone() (*LayoutDSL, error) {
	new := LayoutDSL{}
	bytes, err := jsoniter.Marshal(layout)
	if err != nil {
		return nil, err
	}
	err = jsoniter.Unmarshal(bytes, &new)
	if err != nil {
		return nil, err
	}
	return &new, nil
}
