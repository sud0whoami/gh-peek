// Package table provides a declarative column-width sizer for terminal tables.
//
// Callers define a Table with a slice of Col descriptors (Min, Max, Ideal,
// Elastic, Align) and call Layout to receive realized column widths. When row
// data is passed to Layout, Ideal is automatically derived as the maximum
// visual width of the title and every cell, capped at Max. This "auto-ideal"
// feature prevents any single column from consuming disproportionate slack.
//
// Exactly one column should set Elastic=true; that column absorbs leftover
// space (up to Max) and is the first to shrink when the table is too wide.
package table
