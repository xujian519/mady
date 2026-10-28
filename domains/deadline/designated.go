package deadline

import (
	"fmt"
	"time"
)

// NoticeKind 是启动指定期限的通知书种类。
type NoticeKind string

const (
	// NoticeRejectionDecision 表示驳回决定（起算复审请求期限）。
	NoticeRejectionDecision NoticeKind = "rejection-decision"
	// NoticeOfficeActionFirst 表示第一次审查意见通知书。
	NoticeOfficeActionFirst NoticeKind = "office-action-first"
	// NoticeOfficeActionSubsequent 表示后续审查意见通知书。
	NoticeOfficeActionSubsequent NoticeKind = "office-action-subsequent"
	// NoticeReexamination 表示复审通知书。
	NoticeReexamination NoticeKind = "reexamination-notice"
	// NoticeInvalidationTransfer 表示无效宣告请求转送文件。
	NoticeInvalidationTransfer NoticeKind = "invalidation-transfer"
	// NoticeGrant 表示授予专利权通知（办理登记手续）。
	NoticeGrant NoticeKind = "grant-notice"
)

// NoticeStage 是第一次审查意见通知书所在程序阶段。
type NoticeStage string

const (
	// StageSubstantive 表示实质审查程序（指定期限 4 个月）。
	StageSubstantive NoticeStage = "substantive"
	// StagePreliminary 表示初步审查程序（指定期限 2 个月）。
	StagePreliminary NoticeStage = "preliminary"
)

// noticeSpec 是通知书对应的期限规则。
type noticeSpec struct {
	label       string
	legalBasis  string
	months      int
	stageMonths map[NoticeStage]int
}

// noticeSpecs 是各类通知书的指定期限。
//
// 数字来自《专利审查指南》第五部分第七章 2.1 节与《专利法实施细则》相关条文；
// 通知书自身指定的期限优先于默认值。
var noticeSpecs = map[NoticeKind]noticeSpec{
	NoticeRejectionDecision: {
		label:      "请求复审",
		legalBasis: "专利法第41条第1款",
		months:     3,
	},
	NoticeOfficeActionFirst: {
		label:      "答复第一次审查意见通知书",
		legalBasis: "专利审查指南第五部分第七章第2.1节",
		months:     4,
		stageMonths: map[NoticeStage]int{
			StageSubstantive: 4,
			StagePreliminary: 2,
		},
	},
	NoticeOfficeActionSubsequent: {
		label:      "答复后续审查意见通知书",
		legalBasis: "专利审查指南第五部分第七章第2.1节",
		months:     2,
	},
	NoticeReexamination: {
		label:      "答复复审通知书",
		legalBasis: "专利法实施细则第67条",
		months:     1,
	},
	NoticeInvalidationTransfer: {
		label:      "答复无效宣告请求",
		legalBasis: "专利法实施细则第72条、专利审查指南第四部分第三章第4.4.1节",
		months:     1,
	},
	NoticeGrant: {
		label:      "办理登记手续",
		legalBasis: "专利法实施细则第111条",
		months:     2,
	},
}

// Notice 是一条通知书记录。
type Notice struct {
	// Kind 是通知书种类。
	Kind NoticeKind
	// Delivery 是该通知书的送达记录。
	Delivery DeliveryRequest
	// DesignatedMonths 是通知书自身指定的期限月数，优先于默认值。
	DesignatedMonths *int
	// Stage 是第一次审查意见通知书所在程序阶段；其他种类忽略。
	Stage NoticeStage
}

// Deadline 是一条期限的计算结果。
type Deadline struct {
	// ID 是期限标识。
	ID string
	// Label 是期限名称。
	Label string
	// LegalBasis 是法定依据。
	LegalBasis string
	// DeliveryDate 是送达日。
	DeliveryDate time.Time
	// DeliveryBasis 是送达日的判定依据。
	DeliveryBasis string
	// PeriodEnd 是按期限长度算出的届满日（顺延前）。
	PeriodEnd time.Time
	// End 是顺延后的实际届满日。
	End time.Time
	// Extended 表示届满日是否因休假日顺延。
	Extended bool
	// Months 是期限月数。
	Months int
}

// NoticeDeadline 计算一条通知书触发的指定期限。
func NoticeDeadline(notice Notice, calendar *WorkCalendar) (Deadline, error) {
	spec, ok := noticeSpecs[notice.Kind]
	if !ok {
		return Deadline{}, fmt.Errorf("deadline: 未知通知书种类 %q", notice.Kind)
	}
	delivered, err := notice.Delivery.Resolve()
	if err != nil {
		return Deadline{}, err
	}
	months := spec.months
	if notice.DesignatedMonths != nil && *notice.DesignatedMonths > 0 {
		months = *notice.DesignatedMonths
	} else if notice.Kind == NoticeOfficeActionFirst && notice.Stage != "" {
		if stageMonths, ok := spec.stageMonths[notice.Stage]; ok {
			months = stageMonths
		}
	}
	periodEnd := AddMonths(delivered.Date, months)
	end, extended, err := extend(periodEnd, calendar)
	if err != nil {
		return Deadline{}, err
	}
	return Deadline{
		ID:            string(notice.Kind),
		Label:         spec.label,
		LegalBasis:    spec.legalBasis,
		DeliveryDate:  delivered.Date,
		DeliveryBasis: delivered.Basis,
		PeriodEnd:     periodEnd,
		End:           end,
		Extended:      extended,
		Months:        months,
	}, nil
}

// extend 在必要时把届满日顺延到下一个工作日。
func extend(periodEnd time.Time, calendar *WorkCalendar) (time.Time, bool, error) {
	if calendar == nil {
		loaded, err := BundledWorkCalendar()
		if err != nil {
			return time.Time{}, false, err
		}
		calendar = loaded
	}
	end, err := calendar.ExtendToWorkday(periodEnd)
	if err != nil {
		return time.Time{}, false, err
	}
	return end, DaysBetween(periodEnd, end) > 0, nil
}

// PaymentWindow 是申请费缴纳期限。
//
// 《专利法实施细则》第112条第1款规定两个起算方式：「自申请日起 2 个月」与
// 「自收到受理通知书之日起 15 日」。两者是**择先适用**——以较早届满者为准。
// 只按其中一个计算（尤其是只按较晚者）会把届满日算晚，导致逾期。
type PaymentWindow struct {
	// FromFiling 是自申请日起 2 个月的届满日（顺延后）。
	FromFiling time.Time
	// FromNotice 是自收到受理通知书起 15 日的届满日（顺延后）；未提供受理日时为 nil。
	FromNotice *time.Time
	// End 是实际届满日：两种起算方式中较早者。
	End time.Time
	// Basis 是取该届满日的依据说明。
	Basis string
	// Extended 表示届满日是否因休假日顺延。
	Extended bool
}

// ApplicationFeeWindow 计算申请费缴纳期限，两种起算方式取较早届满者。
//
// 两个届满日各自按《专利法实施细则》第五条顺延后再比较：期限届满日遇休假日
// 顺延是法定规则，对任一计算方式都适用。
func ApplicationFeeWindow(filingDate time.Time, noticeReceipt *time.Time, calendar *WorkCalendar) (PaymentWindow, error) {
	fromFilingRaw := AddMonths(filingDate, 2)
	fromFiling, fromFilingExtended, err := extend(fromFilingRaw, calendar)
	if err != nil {
		return PaymentWindow{}, err
	}

	window := PaymentWindow{FromFiling: fromFiling, Extended: fromFilingExtended}
	if noticeReceipt == nil {
		window.End = fromFiling
		window.Basis = "自申请日起2个月（未提供受理通知书收到日，无法比较择先）"
		return window, nil
	}

	fromNoticeRaw := AddDays(*noticeReceipt, 15)
	fromNotice, fromNoticeExtended, err := extend(fromNoticeRaw, calendar)
	if err != nil {
		return PaymentWindow{}, err
	}
	window.FromNotice = &fromNotice

	switch {
	case fromFiling.Before(fromNotice):
		window.End = fromFiling
		window.Basis = "自申请日起2个月与自收到受理通知书起15日，取较早者：自申请日起2个月"
		window.Extended = fromFilingExtended
	case fromNotice.Before(fromFiling):
		window.End = fromNotice
		window.Basis = "自申请日起2个月与自收到受理通知书起15日，取较早者：自收到受理通知书起15日"
		window.Extended = fromNoticeExtended
	default:
		window.End = fromFiling
		window.Basis = "两种起算方式的届满日相同"
		window.Extended = fromFilingExtended || fromNoticeExtended
	}
	return window, nil
}
