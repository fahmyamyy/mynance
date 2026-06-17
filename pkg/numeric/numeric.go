package numeric

import (
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

const Scale = 10

func Parse(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("numeric.Parse(%q): %w", s, err)
	}
	if !n.Valid {
		return pgtype.Numeric{}, fmt.Errorf("numeric.Parse(%q): not a valid number", s)
	}
	return n, nil
}

func Zero() pgtype.Numeric {
	z, _ := Parse("0")
	return z
}

func String(n pgtype.Numeric) string {
	if !n.Valid {
		return "0"
	}
	r := toRat(n)
	return r.FloatString(Scale)
}

func Mul(a, b pgtype.Numeric) pgtype.Numeric {
	return fromRat(new(big.Rat).Mul(toRat(a), toRat(b)))
}

func Sub(a, b pgtype.Numeric) pgtype.Numeric {
	return fromRat(new(big.Rat).Sub(toRat(a), toRat(b)))
}

func Add(a, b pgtype.Numeric) pgtype.Numeric {
	return fromRat(new(big.Rat).Add(toRat(a), toRat(b)))
}

func Neg(n pgtype.Numeric) pgtype.Numeric {
	return fromRat(new(big.Rat).Neg(toRat(n)))
}

func Cmp(a, b pgtype.Numeric) int {
	return toRat(a).Cmp(toRat(b))
}

func CmpString(a pgtype.Numeric, s string) (int, error) {
	other, err := Parse(s)
	if err != nil {
		return 0, err
	}
	return Cmp(a, other), nil
}

func toRat(n pgtype.Numeric) *big.Rat {
	if !n.Valid || n.Int == nil {
		return new(big.Rat)
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp == 0 {
		return r
	}
	if n.Exp > 0 {
		pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)
		return r.Mul(r, new(big.Rat).SetInt(pow))
	}
	pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil)
	return r.Quo(r, new(big.Rat).SetInt(pow))
}

func fromRat(r *big.Rat) pgtype.Numeric {
	s := r.FloatString(Scale)
	n, _ := Parse(s)
	return n
}
