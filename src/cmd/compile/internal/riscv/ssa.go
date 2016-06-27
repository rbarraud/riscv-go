// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package riscv

import (
	"log"

	"cmd/compile/internal/gc"
	"cmd/compile/internal/ssa"
	"cmd/internal/obj"
	"cmd/internal/obj/riscv"
)

// ssaRegToReg maps ssa register numbers to obj register numbers.
var ssaRegToReg = []int16{
	riscv.REG_X0,
	riscv.REG_X1,
	riscv.REG_X2,
	riscv.REG_X3,
	riscv.REG_X4,
	riscv.REG_X5,
	riscv.REG_X6,
	riscv.REG_X7,
	riscv.REG_X8,
	riscv.REG_X9,
	riscv.REG_X10,
	riscv.REG_X11,
	riscv.REG_X12,
	riscv.REG_X13,
	riscv.REG_X14,
	riscv.REG_X15,
	riscv.REG_X16,
	riscv.REG_X17,
	riscv.REG_X18,
	riscv.REG_X19,
	riscv.REG_X20,
	riscv.REG_X21,
	riscv.REG_X22,
	riscv.REG_X23,
	riscv.REG_X24,
	riscv.REG_X25,
	riscv.REG_X26,
	riscv.REG_X27,
	riscv.REG_X28,
	riscv.REG_X29,
	riscv.REG_X30,
	riscv.REG_X31,
}

func loadByType(t ssa.Type) obj.As {
	width := t.Size()
	if t.IsFloat() {
		panic("float unsupported")
	}

	switch width {
	case 1:
		return riscv.AMOVB
	case 2:
		return riscv.AMOVH
	case 4:
		return riscv.AMOVW
	case 8:
		return riscv.AMOV
	}

	panic("bad store type")
}

// markMoves marks any MOVXconst ops that need to avoid clobbering flags.
func ssaMarkMoves(s *gc.SSAGenState, b *ssa.Block) {
	log.Printf("TODO: ssaMarkMoves")
}

func ssaGenValue(s *gc.SSAGenState, v *ssa.Value) {
	s.SetLineno(v.Line)

	switch v.Op {
	case ssa.OpInitMem:
		// memory arg needs no code
	case ssa.OpArg:
		// input args need no code
	case ssa.OpPhi:
		gc.CheckLoweredPhi(v)
	case ssa.OpCopy, ssa.OpRISCVMOVconvert:
		if v.Type.IsMemory() {
			return
		}
		rs := gc.SSARegNum(v.Args[0])
		rd := gc.SSARegNum(v)
		if rs == rd {
			return
		}
		if v.Type.IsFloat() {
			v.Fatalf("OpCopy float not implemented")
		}
		p := gc.Prog(riscv.AMOV)
		p.From.Type = obj.TYPE_REG
		p.From.Reg = rs
		p.To.Type = obj.TYPE_REG
		p.To.Reg = rd
	case ssa.OpLoadReg:
		if v.Type.IsFlags() {
			v.Unimplementedf("load flags not implemented: %v", v.LongString())
			return
		}
		p := gc.Prog(loadByType(v.Type))
		n, off := gc.AutoVar(v.Args[0])
		p.From.Type = obj.TYPE_MEM
		p.From.Node = n
		p.From.Sym = gc.Linksym(n.Sym)
		p.From.Offset = off
		if n.Class == gc.PPARAM || n.Class == gc.PPARAMOUT {
			p.From.Name = obj.NAME_PARAM
			p.From.Offset += n.Xoffset
		} else {
			p.From.Name = obj.NAME_AUTO
		}
		p.To.Type = obj.TYPE_REG
		p.To.Reg = gc.SSARegNum(v)
	case ssa.OpVarDef:
		gc.Gvardef(v.Aux.(*gc.Node))
	case ssa.OpVarKill:
		gc.Gvarkill(v.Aux.(*gc.Node))
	case ssa.OpVarLive:
		gc.Gvarlive(v.Aux.(*gc.Node))
	case ssa.OpSP, ssa.OpSB:
		// nothing to do
	case ssa.OpRISCVADD, ssa.OpRISCVSUB, ssa.OpRISCVXOR, ssa.OpRISCVOR, ssa.OpRISCVAND,
		ssa.OpRISCVSLT, ssa.OpRISCVSLTU:
		r := gc.SSARegNum(v)
		r1 := gc.SSARegNum(v.Args[0])
		r2 := gc.SSARegNum(v.Args[1])
		p := gc.Prog(v.Op.Asm())
		p.From.Type = obj.TYPE_REG
		p.From.Reg = r2
		p.From3 = &obj.Addr{
			Type: obj.TYPE_REG,
			Reg:  r1,
		}
		p.To.Type = obj.TYPE_REG
		p.To.Reg = r
	case ssa.OpRISCVADDI, ssa.OpRISCVXORI, ssa.OpRISCVORI, ssa.OpRISCVANDI, ssa.OpRISCVSLLI:
		p := gc.Prog(v.Op.Asm())
		p.From.Type = obj.TYPE_CONST
		p.From.Offset = v.AuxInt
		p.From3 = &obj.Addr{Type: obj.TYPE_REG, Reg: gc.SSARegNum(v.Args[0])}
		p.To.Type = obj.TYPE_REG
		p.To.Reg = gc.SSARegNum(v)
	case ssa.OpRISCVMOVBconst, ssa.OpRISCVMOVWconst, ssa.OpRISCVMOVLconst, ssa.OpRISCVMOVQconst:
		p := gc.Prog(v.Op.Asm())
		p.From.Type = obj.TYPE_CONST
		p.From.Offset = v.AuxInt
		p.To.Type = obj.TYPE_REG
		p.To.Reg = gc.SSARegNum(v)
	case ssa.OpRISCVMOVmem:
		p := gc.Prog(v.Op.Asm())
		p.From.Type = obj.TYPE_ADDR
		p.To.Type = obj.TYPE_REG
		p.To.Reg = gc.SSARegNum(v)

		var wantreg string
		// MOVW $sym+off(base), R
		switch v.Aux.(type) {
		default:
			v.Fatalf("aux is of unknown type %T", v.Aux)
		case *ssa.ExternSymbol:
			wantreg = "SB"
			gc.AddAux(&p.From, v)
		case *ssa.ArgSymbol, *ssa.AutoSymbol:
			wantreg = "SP"
			gc.AddAux(&p.From, v)
		case nil:
			// No sym, just MOVW $off(SP), R
			wantreg = "SP"
			p.From.Reg = riscv.REG_SP
			p.From.Offset = v.AuxInt
		}
		// TODO: once a version of dev.ssa is merged that contains gc.SSAReg,
		// uncomment this check.
		// if reg := gc.SSAReg(v.Args[0]); reg.Name() != wantreg {
		// 	v.Fatalf("bad reg %s for symbol type %T, want %s", reg.Name(), v.Aux, wantreg)
		// }
		_ = wantreg
	case ssa.OpRISCVMOVload:
		p := gc.Prog(v.Op.Asm())
		p.From.Type = obj.TYPE_MEM
		p.From.Reg = gc.SSARegNum(v.Args[0])
		gc.AddAux(&p.From, v)
		p.To.Type = obj.TYPE_REG
		p.To.Reg = gc.SSARegNum(v)
	case ssa.OpRISCVMOVstore:
		p := gc.Prog(v.Op.Asm())
		p.From.Type = obj.TYPE_REG
		p.From.Reg = gc.SSARegNum(v.Args[1])
		p.To.Type = obj.TYPE_MEM
		p.To.Reg = gc.SSARegNum(v.Args[0])
		gc.AddAux(&p.To, v)
	case ssa.OpRISCVSEQZ, ssa.OpRISCVSNEZ:
		p := gc.Prog(v.Op.Asm())
		p.From.Type = obj.TYPE_REG
		p.From.Reg = gc.SSARegNum(v.Args[0])
		p.To.Type = obj.TYPE_REG
		p.To.Reg = gc.SSARegNum(v)
	case ssa.OpRISCVBEQ, ssa.OpRISCVBNE, ssa.OpRISCVBLT, ssa.OpRISCVBLTU, ssa.OpRISCVBGE, ssa.OpRISCVBGEU:
		// These are flag pseudo-ops used as control values for conditional branch blocks.
		// See the discussion in RISCVOps.
		// The actual conditional branch instruction will be issued in ssaGenBlock.
	case ssa.OpRISCVLoweredNilCheck:
		log.Printf("TODO: LoweredNilCheck")
	case ssa.OpRISCVLoweredExitProc:
		// MOV rc, A0
		p := gc.Prog(riscv.AMOV)
		p.From.Type = obj.TYPE_REG
		p.From.Reg = gc.SSARegNum(v.Args[0])
		p.To.Type = obj.TYPE_REG
		p.To.Reg = riscv.REG_A0
		// MOV $SYS_EXIT_GROUP, A7
		p = gc.Prog(riscv.AMOV)
		p.From.Type = obj.TYPE_CONST
		p.From.Offset = 94 // SYS_EXIT_GROUP
		p.To.Type = obj.TYPE_REG
		p.To.Reg = riscv.REG_A7
		// SCALL
		p = gc.Prog(riscv.ASCALL)
	default:
		v.Fatalf("Unhandled op %v", v.Op)
	}
}

func ssaGenBlock(s *gc.SSAGenState, b, next *ssa.Block) {
	s.SetLineno(b.Line)

	switch b.Kind {
	case ssa.BlockPlain, ssa.BlockCall, ssa.BlockCheck:
		if b.Succs[0].Block() != next {
			p := gc.Prog(obj.AJMP)
			p.To.Type = obj.TYPE_BRANCH
			s.Branches = append(s.Branches, gc.Branch{P: p, B: b.Succs[0].Block()})
		}
	case ssa.BlockExit:
		gc.Prog(obj.AUNDEF)
	case ssa.BlockRet:
		gc.Prog(obj.ARET)
	case ssa.BlockRISCVBRANCH:
		// Conditional branch. The control value tells us what kind.
		v := b.Control
		// Double-check that the value is exactly where we expect it.
		if v.Block != b {
			gc.Fatalf("control value in the wrong block %v, want %v: %v", v.Block, b, v.LongString())
		}
		if v != b.Values[len(b.Values)-1] {
			gc.Fatalf("badly scheduled control value for block %v: %v", b, v.LongString())
		}
		p := gc.Prog(v.Op.Asm())
		p.To.Type = obj.TYPE_BRANCH
		p.Reg = gc.SSARegNum(v.Args[1])
		p.From.Type = obj.TYPE_REG
		p.From.Reg = gc.SSARegNum(v.Args[0])
		switch next {
		case b.Succs[0].Block():
			p.As = riscv.InvertBranch(p.As)
			s.Branches = append(s.Branches, gc.Branch{P: p, B: b.Succs[1].Block()})
		case b.Succs[1].Block():
			s.Branches = append(s.Branches, gc.Branch{P: p, B: b.Succs[0].Block()})
		default:
			s.Branches = append(s.Branches, gc.Branch{P: p, B: b.Succs[0].Block()})
			q := gc.Prog(obj.AJMP)
			q.To.Type = obj.TYPE_BRANCH
			s.Branches = append(s.Branches, gc.Branch{P: q, B: b.Succs[1].Block()})
		}

	default:
		b.Fatalf("Unhandled kind %v", b.Kind)
	}
}
