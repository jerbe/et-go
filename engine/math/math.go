package math

import (
	"fmt"
	stdmath "math"
)

// Vector3 表示三维浮点向量。
type Vector3 struct {
	X float64
	Y float64
	Z float64
}

var (
	// Vector3Zero 表示零向量。
	Vector3Zero = Vector3{}
	// Vector3Forward 表示默认前方向。
	Vector3Forward = Vector3{X: 0, Y: 0, Z: 1}
)

// Zero 返回零向量。
func Zero() Vector3 {
	return Vector3Zero
}

// Add 返回两个向量的和。
func (v Vector3) Add(other Vector3) Vector3 {
	return Vector3{
		X: v.X + other.X,
		Y: v.Y + other.Y,
		Z: v.Z + other.Z,
	}
}

// Sub 返回两个向量的差。
func (v Vector3) Sub(other Vector3) Vector3 {
	return Vector3{
		X: v.X - other.X,
		Y: v.Y - other.Y,
		Z: v.Z - other.Z,
	}
}

// Mul 返回向量按标量缩放后的结果。
func (v Vector3) Mul(scale float64) Vector3 {
	return Vector3{
		X: v.X * scale,
		Y: v.Y * scale,
		Z: v.Z * scale,
	}
}

// Distance 返回两个向量的欧氏距离。
func (v Vector3) Distance(other Vector3) float64 {
	diff := v.Sub(other)
	return stdmath.Sqrt(diff.X*diff.X + diff.Y*diff.Y + diff.Z*diff.Z)
}

// Normalize 返回单位化后的向量。零向量返回零向量。
func (v Vector3) Normalize() Vector3 {
	length := v.Distance(Vector3Zero)
	if length == 0 {
		return Vector3Zero
	}
	return v.Mul(1 / length)
}

// String 返回便于调试的文本表示。
func (v Vector3) String() string {
	return fmt.Sprintf("(%.4f, %.4f, %.4f)", v.X, v.Y, v.Z)
}

// Lerp 返回两个向量的线性插值。
func Lerp(a, b Vector3, t float64) Vector3 {
	return Vector3{
		X: a.X + (b.X-a.X)*t,
		Y: a.Y + (b.Y-a.Y)*t,
		Z: a.Z + (b.Z-a.Z)*t,
	}
}

// Quaternion 表示四元数旋转。
type Quaternion struct {
	X float64
	Y float64
	Z float64
	W float64
}

var (
	// QuaternionIdentity 表示单位四元数。
	QuaternionIdentity = Quaternion{X: 0, Y: 0, Z: 0, W: 1}
)

// Identity 返回单位四元数。
func Identity() Quaternion {
	return QuaternionIdentity
}

// String 返回四元数调试文本。
func (q Quaternion) String() string {
	return fmt.Sprintf("(%.4f, %.4f, %.4f, %.4f)", q.X, q.Y, q.Z, q.W)
}

// Slerp 返回两个四元数的球面线性插值。
func Slerp(a, b Quaternion, t float64) Quaternion {
	if t <= 0 {
		return normalizeQuaternion(a)
	}
	if t >= 1 {
		return normalizeQuaternion(b)
	}

	left := normalizeQuaternion(a)
	right := normalizeQuaternion(b)

	dot := quaternionDot(left, right)
	if dot < 0 {
		right = Quaternion{
			X: -right.X,
			Y: -right.Y,
			Z: -right.Z,
			W: -right.W,
		}
		dot = -dot
	}

	if dot > 0.9995 {
		return normalizeQuaternion(Quaternion{
			X: left.X + (right.X-left.X)*t,
			Y: left.Y + (right.Y-left.Y)*t,
			Z: left.Z + (right.Z-left.Z)*t,
			W: left.W + (right.W-left.W)*t,
		})
	}

	theta0 := stdmath.Acos(dot)
	sinTheta0 := stdmath.Sin(theta0)
	theta := theta0 * t
	sinTheta := stdmath.Sin(theta)

	s0 := stdmath.Cos(theta) - dot*sinTheta/sinTheta0
	s1 := sinTheta / sinTheta0

	return normalizeQuaternion(Quaternion{
		X: s0*left.X + s1*right.X,
		Y: s0*left.Y + s1*right.Y,
		Z: s0*left.Z + s1*right.Z,
		W: s0*left.W + s1*right.W,
	})
}

// QuaternionForward 根据四元数计算朝向向量。
func QuaternionForward(q Quaternion) Vector3 {
	normalized := normalizeQuaternion(q)
	return Vector3{
		X: 2 * (normalized.X*normalized.Z + normalized.W*normalized.Y),
		Y: 2 * (normalized.Y*normalized.Z - normalized.W*normalized.X),
		Z: 1 - 2*(normalized.X*normalized.X+normalized.Y*normalized.Y),
	}.Normalize()
}

// LookRotation 根据朝向向量构造四元数。
func LookRotation(forward Vector3) Quaternion {
	f := forward.Normalize()
	if f == Vector3Zero {
		return QuaternionIdentity
	}

	up := Vector3{X: 0, Y: 1, Z: 0}
	right := cross(up, f).Normalize()
	if right == Vector3Zero {
		right = Vector3{X: 1, Y: 0, Z: 0}
	}
	up = cross(f, right).Normalize()


	m00, m01, m02 := right.X, up.X, f.X
	m10, m11, m12 := right.Y, up.Y, f.Y
	m20, m21, m22 := right.Z, up.Z, f.Z

	trace := m00 + m11 + m22
	if trace > 0 {
		s := stdmath.Sqrt(trace+1.0) * 2
		return normalizeQuaternion(Quaternion{
			X: (m21 - m12) / s,
			Y: (m02 - m20) / s,
			Z: (m10 - m01) / s,
			W: 0.25 * s,
		})
	}

	if m00 > m11 && m00 > m22 {
		s := stdmath.Sqrt(1.0+m00-m11-m22) * 2
		return normalizeQuaternion(Quaternion{
			X: 0.25 * s,
			Y: (m01 + m10) / s,
			Z: (m02 + m20) / s,
			W: (m21 - m12) / s,
		})
	}

	if m11 > m22 {
		s := stdmath.Sqrt(1.0+m11-m00-m22) * 2
		return normalizeQuaternion(Quaternion{
			X: (m01 + m10) / s,
			Y: 0.25 * s,
			Z: (m12 + m21) / s,
			W: (m02 - m20) / s,
		})
	}

	s := stdmath.Sqrt(1.0+m22-m00-m11) * 2
	return normalizeQuaternion(Quaternion{
		X: (m02 + m20) / s,
		Y: (m12 + m21) / s,
		Z: 0.25 * s,
		W: (m10 - m01) / s,
	})
}

// QuaternionDistance 返回两个四元数的距离。
func QuaternionDistance(a, b Quaternion) float64 {
	left := normalizeQuaternion(a)
	right := normalizeQuaternion(b)

	diff := stdmath.Sqrt(
		(left.X-right.X)*(left.X-right.X) +
			(left.Y-right.Y)*(left.Y-right.Y) +
			(left.Z-right.Z)*(left.Z-right.Z) +
			(left.W-right.W)*(left.W-right.W),
	)
	negDiff := stdmath.Sqrt(
		(left.X+right.X)*(left.X+right.X) +
			(left.Y+right.Y)*(left.Y+right.Y) +
			(left.Z+right.Z)*(left.Z+right.Z) +
			(left.W+right.W)*(left.W+right.W),
	)
	if negDiff < diff {
		return negDiff
	}
	return diff
}

func normalizeQuaternion(q Quaternion) Quaternion {
	lengthSquared := q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W
	if lengthSquared == 0 {
		return QuaternionIdentity
	}
	inverse := 1 / stdmath.Sqrt(lengthSquared)
	return Quaternion{
		X: q.X * inverse,
		Y: q.Y * inverse,
		Z: q.Z * inverse,
		W: q.W * inverse,
	}
}

func quaternionDot(a, b Quaternion) float64 {
	return a.X*b.X + a.Y*b.Y + a.Z*b.Z + a.W*b.W
}

func cross(a, b Vector3) Vector3 {
	return Vector3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}
