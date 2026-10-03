package material

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// Component themes: one type per component (Flutter's XxxThemeData), held
// in Theme and accepted by the widget's Style field. Every field is
// optional; see theme_style.go for what "unset" means and the precedence.

// ButtonStyle styles the common buttons (Elevated, Filled, FilledTonal,
// Outlined, Text), icon buttons and segmented buttons. A few fields only
// make sense for some of them:
//
//   - Selected* colors: toggle icon buttons and selected segments;
//   - Padding, TextStyle: common and segmented buttons (an icon button has
//     no label; size it with MinWidth / MinHeight);
//   - Elevation, MinWidth: common and icon buttons.
//
// An outline needs a color and a width: set only SideColor for a 1px
// line, or only SideWidth for the default color.
type ButtonStyle struct {
	BackgroundColor geom.Color
	ForegroundColor geom.Color // label and icon
	IconColor       geom.Color // default: ForegroundColor
	// OverlayColor tints hover, focus and pressed states (default:
	// ForegroundColor).
	OverlayColor            geom.Color
	DisabledBackgroundColor geom.Color
	DisabledForegroundColor geom.Color
	// Selected colors apply to toggle icon buttons and selected segments.
	SelectedBackgroundColor geom.Color
	SelectedForegroundColor geom.Color
	ShadowColor             geom.Color

	SideColor geom.Color // outline
	SideWidth *float32
	Radius    *float32
	Elevation *int // M3 level 0–5
	Padding   *geom.EdgeInsets
	MinWidth  *float32
	MinHeight *float32
	IconSize  *float32
	TextStyle text.Style
}

// FloatingActionButtonTheme styles FloatingActionButton.
type FloatingActionButtonTheme struct {
	BackgroundColor geom.Color
	ForegroundColor geom.Color
	OverlayColor    geom.Color
	Elevation       *int
	Radius          *float32 // all sizes
	IconSize        *float32
	// Extended FABs (with a label).
	ExtendedTextStyle text.Style
	ExtendedPadding   *geom.EdgeInsets
}

// CardTheme styles Card. As for buttons, a BorderColor alone draws a 1px
// border and a BorderWidth alone uses the default outline color.
type CardTheme struct {
	Color        geom.Color
	OverlayColor geom.Color // tappable cards
	ShadowColor  geom.Color
	BorderColor  geom.Color
	BorderWidth  *float32
	Radius       *float32
	Elevation    *int
	Margin       *geom.EdgeInsets
}

// DividerTheme styles Divider and VerticalDivider.
type DividerTheme struct {
	Color     geom.Color
	Thickness *float32
	Space     *float32 // total height (width for vertical)
	Indent    *float32
	EndIndent *float32
}

// ListTileTheme styles ListTile and the tiles built on it.
type ListTileTheme struct {
	TileColor         geom.Color
	SelectedTileColor geom.Color
	TextColor         geom.Color // title
	SubtitleColor     geom.Color
	IconColor         geom.Color // leading and trailing
	SelectedColor     geom.Color // title and icons when selected
	OverlayColor      geom.Color
	TitleTextStyle    text.Style
	SubtitleTextStyle text.Style
	TrailingTextStyle text.Style
	ContentPadding    *geom.EdgeInsets
	HorizontalGap     *float32 // between leading, title and trailing
	MinHeight         *float32
	Radius            *float32
}

// ExpansionTileTheme styles ExpansionTile.
type ExpansionTileTheme struct {
	BackgroundColor          geom.Color // expanded
	CollapsedBackgroundColor geom.Color
	IconColor                geom.Color // expanded
	CollapsedIconColor       geom.Color
	TextColor                geom.Color // title when expanded
	CollapsedTextColor       geom.Color
	ChildrenPadding          *geom.EdgeInsets
}

// BadgeTheme styles Badge.
type BadgeTheme struct {
	BackgroundColor geom.Color
	TextColor       geom.Color
	SmallSize       *float32 // dot diameter
	LargeSize       *float32 // height with a label
	TextStyle       text.Style
	Padding         *geom.EdgeInsets
}

// CircleAvatarTheme styles CircleAvatar.
type CircleAvatarTheme struct {
	BackgroundColor geom.Color
	ForegroundColor geom.Color
	Radius          *float32
	TextStyle       text.Style
}

// TooltipTheme styles Tooltip.
type TooltipTheme struct {
	Color     geom.Color
	TextStyle text.Style
	Padding   *geom.EdgeInsets
	Radius    *float32
	// WaitDuration is the hover time before it shows (default 500ms).
	WaitDuration time.Duration
}

// BannerTheme styles MaterialBanner.
type BannerTheme struct {
	BackgroundColor  geom.Color
	ContentTextStyle text.Style
	LeadingColor     geom.Color // the icon's circle
	DividerColor     geom.Color
	Padding          *geom.EdgeInsets
}

// ChipTheme styles all chips.
type ChipTheme struct {
	BackgroundColor    geom.Color
	SelectedColor      geom.Color
	DisabledColor      geom.Color
	LabelColor         geom.Color
	SelectedLabelColor geom.Color
	IconColor          geom.Color
	CheckmarkColor     geom.Color
	DeleteIconColor    geom.Color
	SideColor          geom.Color
	SideWidth          *float32
	Radius             *float32
	Elevation          *int // elevated chips
	Height             *float32
	Padding            *geom.EdgeInsets // horizontal
	LabelStyle         text.Style
}

// CheckboxTheme styles Checkbox.
type CheckboxTheme struct {
	FillColor    geom.Color // checked box
	CheckColor   geom.Color // the check mark
	BorderColor  geom.Color // unchecked outline
	ErrorColor   geom.Color
	OverlayColor geom.Color
	Size         *float32 // box side (default 18)
	Radius       *float32
	BorderWidth  *float32
}

// RadioTheme styles Radio.
type RadioTheme struct {
	FillColor       geom.Color // selected
	UnselectedColor geom.Color
	OverlayColor    geom.Color
	Radius          *float32 // outer ring (default 10)
}

// SwitchTheme styles Switch.
type SwitchTheme struct {
	TrackColor         geom.Color // on
	InactiveTrackColor geom.Color
	TrackOutlineColor  geom.Color
	ThumbColor         geom.Color // on
	InactiveThumbColor geom.Color
	PressedThumbColor  geom.Color
	ThumbIconColor     geom.Color
	OverlayColor       geom.Color
}

// SliderTheme styles Slider and RangeSlider.
type SliderTheme struct {
	ActiveTrackColor        geom.Color
	InactiveTrackColor      geom.Color
	ThumbColor              geom.Color
	OverlayColor            geom.Color
	ActiveTickMarkColor     geom.Color
	InactiveTickMarkColor   geom.Color
	ValueIndicatorColor     geom.Color
	ValueIndicatorTextStyle text.Style
	TrackHeight             *float32
	ThumbRadius             *float32
}

// ProgressIndicatorTheme styles the progress indicators.
type ProgressIndicatorTheme struct {
	Color        geom.Color
	TrackColor   geom.Color
	LinearHeight *float32
	StrokeWidth  *float32 // circular
	CircularSize *float32
}

// InputDecorationTheme styles TextField.
type InputDecorationTheme struct {
	Outlined         *bool      // TextField{Outlined: true} is outlined regardless
	FillColor        geom.Color // filled fields
	FocusColor       geom.Color // label, indicator and cursor when focused
	BorderColor      geom.Color // idle indicator / outline
	HoverBorderColor geom.Color
	ErrorColor       geom.Color
	IconColor        geom.Color
	TextStyle        text.Style // the input
	LabelStyle       text.Style
	HintStyle        text.Style
	HelperStyle      text.Style
	Radius           *float32
}

// SearchBarTheme styles SearchBar.
type SearchBarTheme struct {
	BackgroundColor geom.Color
	OverlayColor    geom.Color
	IconColor       geom.Color
	Elevation       *int
	Radius          *float32
	Height          *float32
	TextStyle       text.Style
	HintStyle       text.Style
}

// AppBarTheme styles AppBar.
type AppBarTheme struct {
	BackgroundColor    geom.Color
	ScrolledUnderColor geom.Color
	ForegroundColor    geom.Color // title and leading icon
	ActionsIconColor   geom.Color
	ShadowColor        geom.Color
	Elevation          *int
	Height             *float32 // the toolbar row (default 64)
	CenterTitle        *bool    // AppBarSmall bars (other variants keep theirs)
	TitleTextStyle     text.Style
}

// NavigationBarTheme styles NavigationBar.
type NavigationBarTheme struct {
	BackgroundColor    geom.Color
	IndicatorColor     geom.Color
	IconColor          geom.Color
	SelectedIconColor  geom.Color
	LabelColor         geom.Color
	SelectedLabelColor geom.Color
	OverlayColor       geom.Color
	LabelTextStyle     text.Style
	Height             *float32
}

// NavigationRailTheme styles NavigationRail.
type NavigationRailTheme struct {
	BackgroundColor    geom.Color
	IndicatorColor     geom.Color
	IconColor          geom.Color
	SelectedIconColor  geom.Color
	LabelColor         geom.Color
	SelectedLabelColor geom.Color
	OverlayColor       geom.Color
	LabelTextStyle     text.Style
	Width              *float32
}

// NavigationDrawerTheme styles NavigationDrawer.
type NavigationDrawerTheme struct {
	BackgroundColor    geom.Color
	IndicatorColor     geom.Color
	IconColor          geom.Color
	SelectedIconColor  geom.Color
	LabelColor         geom.Color
	SelectedLabelColor geom.Color
	HeadlineColor      geom.Color
	LabelTextStyle     text.Style
	Width              *float32
	Radius             *float32
	ItemHeight         *float32
}

// TabBarTheme styles TabBar.
type TabBarTheme struct {
	IndicatorColor       geom.Color
	LabelColor           geom.Color // selected
	UnselectedLabelColor geom.Color
	DividerColor         geom.Color
	OverlayColor         geom.Color
	LabelStyle           text.Style
	IndicatorHeight      *float32
	Height               *float32
}

// BottomAppBarTheme styles BottomAppBar.
type BottomAppBarTheme struct {
	Color     geom.Color
	IconColor geom.Color
	Height    *float32
	Padding   *geom.EdgeInsets
}

// DialogTheme styles Dialog, AlertDialog and SimpleDialog.
type DialogTheme struct {
	BackgroundColor  geom.Color
	BarrierColor     geom.Color // the scrim of ShowDialog
	IconColor        geom.Color
	ShadowColor      geom.Color
	TitleTextStyle   text.Style
	ContentTextStyle text.Style
	Elevation        *int
	Radius           *float32
	Padding          *geom.EdgeInsets
}

// BottomSheetTheme styles BottomSheet and ShowModalBottomSheet.
type BottomSheetTheme struct {
	BackgroundColor geom.Color
	BarrierColor    geom.Color
	DragHandleColor geom.Color
	ShadowColor     geom.Color
	Elevation       *int
	Radius          *float32
	MaxWidth        *float32
	ShowDragHandle  *bool
}

// SideSheetTheme styles SideSheet.
type SideSheetTheme struct {
	BackgroundColor      geom.Color // standard sheets
	ModalBackgroundColor geom.Color
	BarrierColor         geom.Color
	DividerColor         geom.Color
	IconColor            geom.Color
	TitleTextStyle       text.Style
	Width                *float32
	Radius               *float32 // modal sheets
}

// SnackBarTheme styles snack bars (ShowSnackBar).
type SnackBarTheme struct {
	BackgroundColor  geom.Color
	ActionTextColor  geom.Color
	CloseIconColor   geom.Color
	ContentTextStyle text.Style
	Elevation        *int
	Radius           *float32
	MaxWidth         *float32
}

// MenuTheme styles menus: ShowMenu, PopupMenuButton, DropdownMenu.
type MenuTheme struct {
	BackgroundColor geom.Color
	TextColor       geom.Color
	IconColor       geom.Color
	SelectedColor   geom.Color // selected item background
	ShadowColor     geom.Color
	TextStyle       text.Style
	Elevation       *int
	Radius          *float32
	ItemHeight      *float32
}

// QuickPickTheme styles QuickPick and the command palette.
type QuickPickTheme struct {
	BackgroundColor geom.Color
	SelectedColor   geom.Color // the highlighted item
	MatchColor      geom.Color // matched characters of an item
	ShadowColor     geom.Color
	TextStyle       text.Style // item labels
	DetailStyle     text.Style // item details
	HintStyle       text.Style // right-aligned hints (shortcuts)
	InputStyle      text.Style // the query
	Elevation       *int
	Radius          *float32
	Width           *float32
	ItemHeight      *float32
}

// DropdownMenuTheme styles the DropdownMenu field (its menu uses MenuTheme).
type DropdownMenuTheme struct {
	TextStyle   text.Style
	LabelStyle  text.Style
	BorderColor geom.Color
	IconColor   geom.Color
	Width       *float32
	Radius      *float32
}

// DataTableTheme styles DataTable.
type DataTableTheme struct {
	HeadingRowColor  geom.Color
	DataRowColor     geom.Color
	SelectedRowColor geom.Color
	DividerColor     geom.Color
	SortIconColor    geom.Color
	HeadingTextStyle text.Style
	DataTextStyle    text.Style
	HeadingRowHeight *float32
	DataRowHeight    *float32
	HorizontalMargin *float32 // cell padding
}

// StepperTheme styles Stepper.
type StepperTheme struct {
	ActiveColor       geom.Color // current and completed step circles
	ActiveIconColor   geom.Color
	InactiveColor     geom.Color
	InactiveIconColor geom.Color
	ErrorColor        geom.Color
	ConnectorColor    geom.Color
	TitleTextStyle    text.Style
	SubtitleTextStyle text.Style
}

// DatePickerTheme styles CalendarDatePicker and ShowDatePicker.
type DatePickerTheme struct {
	BackgroundColor            geom.Color
	HeaderForegroundColor      geom.Color
	HeadlineTextStyle          text.Style
	DayTextStyle               text.Style
	WeekdayTextStyle           text.Style
	DayForegroundColor         geom.Color
	SelectedDayBackgroundColor geom.Color
	SelectedDayForegroundColor geom.Color
	TodayForegroundColor       geom.Color
	TodayBorderColor           geom.Color
}

// TimePickerTheme styles ShowTimePicker.
type TimePickerTheme struct {
	InputBackgroundColor geom.Color
	InputTextStyle       text.Style
	HelpTextStyle        text.Style
	Radius               *float32 // input boxes
}

// CarouselTheme styles Carousel items without their own content color.
type CarouselTheme struct {
	// ItemColors cycle through the placeholder item fills.
	ItemColors [3]geom.Color
	Radius     *float32
}

// TreeViewTheme styles TreeView and FileTree.
type TreeViewTheme struct {
	TextStyle         text.Style
	SelectedTextColor geom.Color
	IconColor         geom.Color
	FolderColor       geom.Color // FileTree folder icons
	SelectedColor     geom.Color // selected row
	HoverColor        geom.Color
	FocusColor        geom.Color
	GuideColor        geom.Color
	ScrollbarColor    geom.Color
	Radius            *float32
}

// SplitViewTheme styles the SplitView drag handles.
type SplitViewTheme struct {
	HandleColor        geom.Color
	HoverHandleColor   geom.Color
	DraggedHandleColor geom.Color
}

// TitleBarTheme styles TitleBar.
type TitleBarTheme struct {
	BackgroundColor geom.Color
	IconColor       geom.Color
	TitleTextStyle  text.Style
	Height          *float32
}
