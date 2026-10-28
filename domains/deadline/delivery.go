package deadline

import (
	"fmt"
	"time"
)

// DeliveryMode 是通知书的送达方式。
type DeliveryMode string

const (
	// DeliveryElectronic 表示电子送达（当前默认方式）。
	DeliveryElectronic DeliveryMode = "electronic"
	// DeliveryPostal 表示邮寄送达。
	DeliveryPostal DeliveryMode = "postal"
	// DeliveryPersonal 表示直接送交。
	DeliveryPersonal DeliveryMode = "personal"
	// DeliveryPublication 表示公告送达。
	DeliveryPublication DeliveryMode = "publication"
)

// DeliveryRequest 是一条送达记录。
//
// 各字段按送达方式取用；缺必需字段时 Resolve 报错，不替调用方猜一个日期。
type DeliveryRequest struct {
	// Mode 是送达方式；零值按电子送达处理。
	Mode DeliveryMode
	// DispatchDate 是发文日（电子送达与邮寄）。
	DispatchDate *time.Time
	// EnteredDate 是文件进入当事人电子系统的日期。
	EnteredDate *time.Time
	// ActualReceiptDate 是当事人举证的实际收到日期。
	ActualReceiptDate *time.Time
	// HandedOverDate 是直接送交的交付日。
	HandedOverDate *time.Time
	// PublicationDate 是公告送达的公告日。
	PublicationDate *time.Time
}

// DeliveryDate 是解析出的送达日与所依据的规则。
type DeliveryDate struct {
	// Date 是送达日。
	Date time.Time
	// Basis 是适用的规则说明，供报告溯源。
	Basis string
}

// Resolve 解析送达日。
//
//   - 电子送达：以进入电子系统日为准；无该证据时，指南推定以发文日为送达日，
//     期限自发文日起算，不再加 15 日（细则第四条 + 指南第五部分第六章 2.3.1）。
//   - 邮寄：有实际收到日则以其为准，否则发文日 + 15 日推定（推定收到日不顺延）。
//   - 直接送交：交付日。
//   - 公告：公告日 + 1 个月。
func (r DeliveryRequest) Resolve() (DeliveryDate, error) {
	mode := r.Mode
	if mode == "" {
		mode = DeliveryElectronic
	}
	switch mode {
	case DeliveryElectronic:
		date := r.EnteredDate
		basis := "专利法实施细则第四条：以进入当事人认可的电子系统的日期为送达日"
		if date == nil {
			date = r.DispatchDate
			basis = "专利法实施细则第四条 + 专利审查指南第五部分第六章2.3.1：电子送达以发文日为送达日，期限自发文日起算（不再加15日）"
		}
		if date == nil {
			return DeliveryDate{}, fmt.Errorf("deadline: 电子送达需要发文日或电子系统进入日")
		}
		return DeliveryDate{Date: DateOnly(*date), Basis: basis}, nil
	case DeliveryPostal:
		if r.ActualReceiptDate != nil {
			return DeliveryDate{
				Date:  DateOnly(*r.ActualReceiptDate),
				Basis: "专利法实施细则第四条：当事人举证证明实际收到日期的，以实际收到日为准",
			}, nil
		}
		if r.DispatchDate == nil {
			return DeliveryDate{}, fmt.Errorf("deadline: 邮寄送达需要发文日或实际收到日")
		}
		return DeliveryDate{
			Date:  AddDays(*r.DispatchDate, 15),
			Basis: "专利法实施细则第四条：邮寄文件自发出之日起满15日推定为收到日（推定收到日遇休假日不顺延）",
		}, nil
	case DeliveryPersonal:
		if r.HandedOverDate == nil {
			return DeliveryDate{}, fmt.Errorf("deadline: 直接送交需要交付日")
		}
		return DeliveryDate{
			Date:  DateOnly(*r.HandedOverDate),
			Basis: "专利法实施细则第四条：直接送交的文件以交付日为送达日",
		}, nil
	case DeliveryPublication:
		if r.PublicationDate == nil {
			return DeliveryDate{}, fmt.Errorf("deadline: 公告送达需要公告日")
		}
		return DeliveryDate{
			Date:  AddMonths(*r.PublicationDate, 1),
			Basis: "专利法实施细则第四条：公告送达自公告之日起满1个月视为已经送达",
		}, nil
	default:
		return DeliveryDate{}, fmt.Errorf("deadline: 未知送达方式 %q", mode)
	}
}
