{ Dhrystone 2.1, Reinhold P. Weicker, 25 May 1988.
  Adapted to ISO 7185. The Berkeley clock calls are constant, so the
  Go benchmark times the process rather than the program's own clock. }
program Dhrystone (input, output);
(***************)

const (* for measurement *)

  MicrosecondsPerClock  = 1000;
  ClocksPerSecond       = 1000;
        (* In Berkeley UNIX Pascal, the function "clock"        *)
        (* returns milliseconds                                 *)
  TooSmallTime          = 2000;
        (* Measurements should last at least 2 seconds          *)

type

  (* Global type definitions *)

  Enumeration           = (Ident1, Ident2, Ident3, Ident4, Ident5);

  OneToThirty           = 1..30;
  OneToFifty            = 1..50;
  CapitalLetter         = 'A'..'Z';

  String30              = packed array [OneToThirty] of char;

  Array1DimInteger      = array [OneToFifty] of integer;
  Array2DimInteger      = array [OneToFifty, OneToFifty] of integer;

  RecordPointer         = ^RecordType;

  RecordType            =
      record
        PointerComp:   RecordPointer;
        case Discr:    Enumeration of
          Ident1:         (* only this variant is used,           *)
                          (* but in some cases discriminant       *)
                          (* checks are necessary                 *)
            (EnumComp:      Enumeration;
             IntComp:       OneToFifty;
             StringComp:    String30);
          Ident2:
            (Enum2Comp:    Enumeration;
             String2Comp:  String30);
          Ident3, Ident4, Ident5:
            (Char1Comp,
             Char2Comp:    char);
      end; (* record *)

var

  (* Ada version: Variables local in Proc_0 *)

  Int1Glob,
  Int2Glob,
  Int3Glob:       OneToFifty;
  CharIndex:      char;
  EnumGlob:       Enumeration;
  String1Glob,
  String2Glob:    String30;

  (* Ada version: Variables global in Pack_1 *)

  PointerGlob,
  NextPointerGlob: RecordPointer;
  IntGlob:         integer;

  BoolGlob:        boolean;
  Char1Glob,
  Char2Glob:       char;
  Array1Glob:      Array1DimInteger;
  Array2Glob:      Array2DimInteger;

  (* Variables for measurement *)

  RunIndex,
  NumberOfRuns,
  BeginClock,
  EndClock,
  SumClocks:            integer;
  Microseconds,         
  DhrystonesPerSecond:  real;
  I:                    integer;

  (* end of variables for measurement *)

procedure Proc1 (    PointerParVal: RecordPointer);     forward;

procedure Proc2 (var IntParRef:     OneToFifty);        forward;

procedure Proc3 (var PointerParRef: RecordPointer);     forward;

procedure Proc4;                                        forward;
  (* without parameters *)

procedure Proc5;                                        forward;
  (* without parameters *)

procedure Proc6 (    EnumParVal:    Enumeration;
                 var EnumParRef:    Enumeration);       forward;

procedure Proc7 (    Int1ParVal,
                     Int2ParVal:    OneToFifty;
                 var IntParRef:     OneToFifty);        forward;

procedure Proc8 (var Array1ParRef:  Array1DimInteger;
                 var Array2ParRef:  Array2DimInteger;
                     Int1ParVal,
                     Int2ParVal:    integer);            forward;

function Func1  (    Char1ParVal,
                     Char2ParVal:   CapitalLetter): 
                                            Enumeration; forward;

function Func2  (var String1ParRef,
                     String2ParRef: String30): 
                                            boolean;      forward;

function Func3  (    EnumParVal:    Enumeration): 
                                            boolean;      forward;


procedure Proc1; (* (PointerParVal: RecordPointer) *)
    (* executed once *)
begin
  with PointerParVal^.PointerComp^ (* = PointerGlobNext *) do
  begin
    PointerParVal^.PointerComp^ := PointerGlob^;
    PointerParVal^.IntComp := 5;
    IntComp := PointerParVal^.IntComp;
    PointerComp := PointerParVal^.PointerComp;
    Proc3 (PointerComp);
      (* PointerParVal^.PointerComp^.PointerComp = PointerGlob^.PointerComp *)
    if Discr = Ident1
    then (* executed *)
    begin
      IntComp := 6;
      Proc6 (PointerParVal^.EnumComp, EnumComp);
      PointerComp := PointerGlob^.PointerComp;
      Proc7 (IntComp, 10, IntComp);
    end (* then *)
    else (* not executed *)
      PointerParVal^ := PointerParVal^.PointerComp^;
  end; (* with *)
end; (* Proc1 *)


procedure Proc2; (* (var IntParRef: OneToFifty) *)
    (* executed once *)
    (* InParRef = 3, becomes 7 *)
var
  IntLoc:  OneToFifty;
  EnumLoc: Enumeration;
begin
  IntLoc := IntParRef + 10;
  repeat (* executed once *)
    if Char1Glob = 'A'
    then (* executed *)
    begin
      IntLoc := IntLoc - 1;
      IntParRef := IntLoc - IntGlob;
      EnumLoc := Ident1;
    end (* if *)
  until EnumLoc = Ident1; (* true *)
end; (* Proc2 *)


procedure Proc3; (* (var PointerParRef: RecordPointer) *)
    (* executed once *)
    (* PointerParRef becomes PointerGlob *)
begin
  if PointerGlob <> nil
  then (* executed *)
    PointerParRef := PointerGlob^.PointerComp;
  Proc7 (10, IntGlob, PointerGlob^.IntComp);
end; (* Proc3 *)


procedure Proc4; (* without parameters *)
    (* executed once *)
var
  BoolLoc: boolean;
begin
  BoolLoc := Char1Glob = 'A';
  BoolGlob := BoolLoc or BoolGlob;
  Char2Glob := 'B';
end; (* Proc4 *)


procedure Proc5; (* without parameters *)
    (* executed once *)
begin
  Char1Glob := 'A';
  BoolGlob := false;
end; (* Proc5 *)


procedure Proc6; (* (    EnumParVal:     Enumeration;
                     var EnumParRef:     Enumeration) *)
    (* executed once *)
    (* EnumParVal = Ident3, EnumParRef becomes Ident2 *)
begin
  EnumParRef := EnumParVal;
  if not Func3 (EnumParVal)
  then (* not executed *)
    EnumParRef := Ident4;
  case EnumParVal of
    Ident1: EnumParRef := Ident1;
    Ident2: if IntGlob > 100
              then EnumParRef := Ident1
              else EnumParRef := Ident4;
    Ident3: EnumParRef := Ident2;    (* executed *)
    Ident4: ;
    Ident5: EnumParRef := Ident3;
  end; (* case *)
end; (* Proc6 *)


procedure Proc7; (* (    Int1ParVal,
                         Int2ParVal:    OneToFifty;
                     var IntParRef:     OneToFifty) *)
    (* executed three times                               *)
    (* first call:      Int1ParVal = 2, Int2ParVal = 3,   *)
    (*                  IntParRef becomes 7               *)
    (* second call:     Int1ParVal = 10, Int2ParVal = 5,  *)
    (*                  IntParRef becomes 17              *)
    (* third call:      Int1ParVal = 6, Int2ParVal = 10,  *)
    (*                  IntParRef becomes 18              *)
var
  IntLoc: OneToFifty;
begin
  IntLoc := Int1ParVal + 2;
  IntParRef := Int2ParVal + IntLoc;
end; (* Proc7 *)


procedure Proc8; (* (var Array1ParRef: Array1DimInteger;
                     var Array2ParRef: Array2DimInteger;
                         Int1ParVal,
                         Int2ParVal:    integer)          *)
    (* executed once  *)
    (* Int1ParVal = 3 *)
    (* Int2ParVal = 7 *)
var
  IntIndex,
  IntLoc:   OneToFifty;
begin
  IntLoc := Int1ParVal + 5;
  Array1ParRef [IntLoc] := Int2ParVal;
  Array1ParRef [IntLoc+1] := Array1ParRef [IntLoc];
  Array1ParRef [IntLoc+30] := IntLoc;
  for IntIndex := IntLoc to IntLoc+1 do
    Array2ParRef [IntLoc, IntIndex] := IntLoc;
  Array2ParRef [IntLoc, IntLoc-1] := Array2ParRef [IntLoc, IntLoc-1] + 1;
  Array2ParRef [IntLoc+20, IntLoc] := Array1ParRef [IntLoc];
  IntGlob := 5;
end; (* Proc8 *)


function Func1; (* (Char1ParVal,
                    Char2ParVal: CapitalLetter): Enumeration *)
    (* executed three times, returns always Ident1              *)
    (* first call:      Char1ParVal = 'H', Char2ParVal = 'R'    *)
    (* second call:     Char1ParVal = 'A', Char2ParVal = 'C'    *)
    (* third call:      Char1ParVal = 'B', Char2ParVal = 'C'    *)
var
  Char1Loc, Char2Loc: CapitalLetter;
begin
  Char1Loc := Char1ParVal;
  Char2Loc := Char1Loc;
  if Char2Loc <> Char2ParVal
  then  (* executed *)
    Func1 := Ident1
  else  (* not executed *)
  begin
    Char1Glob := Char1Loc;
    Func1 := Ident2;
  end;
end; (* Func1 *)


function Func2; (* (var String1ParRef,
                        String2ParRef: String30): boolean *)
    (* executed once, returns false              *)
    (* String1ParRef = 'DHRYSTONE PROGRAM, 1''ST STRING' *)
    (* String2ParRef = 'DHRYSTONE PROGRAM, 2''ND STRING' *)
var
  IntLoc:  OneToThirty;
  CharLoc: CapitalLetter;
begin
  IntLoc := 2;
  while IntLoc <= 2 do (* loop body executed once *)
    if Func1 (String1ParRef[IntLoc],
              String2ParRef[IntLoc+1]) = Ident1
    then (* executed *)
    begin
      CharLoc := 'A';
      IntLoc := IntLoc + 1;
    end; (* if, while *)
  if (CharLoc >= 'W') and (CharLoc < 'Z')
  then (* not executed *)
    IntLoc := 7;
  if CharLoc = 'R'
  then (* not executed *)
    Func2 := true
  else (* executed *)
  begin
    if String1ParRef > String2ParRef
    then (* not executed *)
    begin
      IntLoc := IntLoc + 7;
      IntGlob := IntLoc;
      Func2 := true
    end
    else (* executed *)
      Func2 := false;
  end; (* if CharLoc *)
end; (* Func2 *)


function Func3; (* (EnumParVal: Enumeration): boolean *)
    (* executed once, returns true      *)
    (* EnumParVal = Ident3              *)
var
  EnumLoc:  Enumeration;
begin
  EnumLoc := EnumParVal;
  if EnumLoc = Ident3
  then (* executed *)
    Func3 := true
  else (* not executed *)
    Func3 := false;
end; (* Func3 *)


begin (* main program, corresponds to procedures        *)
      (* Main and Proc_0 in the Ada version             *)

  (* Initializations *)

  new (NextPointerGlob);

  new (PointerGlob);

  PointerGlob^.PointerComp := NextPointerGlob;
  PointerGlob^.Discr       := Ident1;
  PointerGlob^.EnumComp    := Ident3;
  PointerGlob^.IntComp     := 40;
  PointerGlob^.StringComp  := 'DHRYSTONE PROGRAM, SOME STRING';

  String1Glob := 'DHRYSTONE PROGRAM, 1''ST STRING';

  Array2Glob [8,7] := 10;

  writeln;
  writeln ('Dhrystone Benchmark, Version 2.1 (Language: Pascal)');
  writeln;
  writeln ('Please give the number of runs through the benchmark: ');
  readln (NumberOfRuns);
  writeln;
  writeln ('Execution starts, ', NumberOfRuns : 7, ' runs through Dhrystone');

  BeginClock := 0;

  (***************)
  (* Start timer *)
  (***************)
  
  for RunIndex := 1 to NumberOfRuns do
  begin

    Proc5;
    Proc4;
      (* Char1Glob = 'A', Char2Glob = 'B', BoolGlob = false *)
    Int1Glob := 2;
    Int2Glob := 3;
    String2Glob := 'DHRYSTONE PROGRAM, 2''ND STRING';
    EnumGlob := Ident2;
    BoolGlob := not Func2 (String1Glob, String2Glob);
      (* BoolGlob = true *)
    while Int1Glob < Int2Glob do  (* loop body executed once *)
    begin
      Int3Glob := 5 * Int1Glob - Int2Glob;
        (* Int3Glob = 7 *)
      Proc7 (Int1Glob, Int2Glob, Int3Glob);
        (* Int3Glob = 7 *)
      Int1Glob := Int1Glob + 1;
    end; (* while *)
      (* Int1Glob = 3 *)
    Proc8 (Array1Glob, Array2Glob, Int1Glob, Int3Glob);
      (* IntGlob = 5 *)
    Proc1 (PointerGlob);
    for CharIndex := 'A' to Char2Glob do   (* loop body executed twice *)
      if EnumGlob = Func1 (CharIndex, 'C')
        then (* not executed *)
        begin
          Proc6 (Ident1, EnumGlob);
          String2Glob := 'DHRYSTONE PROGRAM, 3''RD STRING';
          Int2Glob := RunIndex;
          IntGlob := RunIndex;
        end;
    (* Int1Glob = 3, Int2Glob = 3, Int3Glob = 7 *)
    Int2Glob := Int2Glob * Int1Glob;
    Int1Glob := Int2Glob div Int3Glob;
    Int2Glob := 7 * (Int2Glob - Int3Glob) - Int1Glob;
      (* Int1Glob = 1, Int2Glob = 13, Int3Glob = 7 *)
    Proc2 (Int1Glob);
      (* Int1Glob = 5 *)

  end; (* for RunIndex *)

  EndClock := 0;

  (**************)
  (* Stop timer *)
  (**************)

  writeln ('Execution ends');
  writeln;
  writeln ('Final values of the variables used in the benchmark:');
  writeln;

  writeln ('IntGlob:                      ', IntGlob : 5);
  writeln ('        should be:                5');
  write ('BoolGlob:                      ');
  if BoolGlob = true
  then
    writeln ('TRUE')
  else
    writeln ('FALSE');
  writeln ('        should be:             TRUE');
  writeln ('Char1Glob:                        ', Char1Glob);
  writeln ('        should be:                A');
  writeln ('Char2Glob:                        ', Char2Glob);
  writeln ('        should be:                B');
  writeln ('Array1Glob [8]:               ', Array1Glob [8] : 5);
  writeln ('        should be:                7');
  writeln ('Array2Glob [8,7]:             ', Array2Glob [8,7] : 5);
  writeln ('        should be:                NumberOfRuns + 10');
  writeln ('PointerGlob^.Discr:           ', ord (PointerGlob^.Discr) : 5);
  writeln ('        should be:                0');
  writeln ('PointerGlob^.EnumComp:        ', ord (PointerGlob^.EnumComp) : 5);
  writeln ('        should be:                2');
  writeln ('PointerGlob^.IntComp  :       ', PointerGlob^.IntComp : 5);
  writeln ('        should be:               17');
  write   ('PointerGlob^.StringComp:     ');
  for I := 1 to 30 do
    write (PointerGlob^.StringComp [I]);
  writeln;
  writeln ('        should be:           DHRYSTONE PROGRAM, SOME STRING');
  writeln ('NextPointerGlob^.Discr:       ', ord (NextPointerGlob^.Discr) : 5);
  writeln ('        should be:                0');
  writeln ('NextPointerGlob^.EnumComp:    ',
                    ord (NextPointerGlob^.EnumComp) : 5);
  writeln ('        should be:                1');
  writeln ('NextPointerGlob^.IntComp:     ', NextPointerGlob^.IntComp : 5);
  writeln ('        should be:               18');
  write   ('NextPointerGlob^.StringComp: ');
  for I := 1 to 30 do
    write (NextPointerGlob^.StringComp [I]);
  writeln;
  writeln ('        should be:           DHRYSTONE PROGRAM, SOME STRING');
  writeln ('Int1Glob:                     ', Int1Glob : 5);
  writeln ('        should be:                5');
  writeln ('Int2Glob:                     ', Int2Glob : 5);
  writeln ('        should be:               13');
  writeln ('Int3Glob:                     ', Int3Glob : 5);
  writeln ('        should be:                7');
  writeln ('EnumGlob:                     ', ord (EnumGlob) : 5);
  writeln ('        should be:                1');
  write   ('String1Glob:                 ');
  for I := 1 to 30 do
    write (String1Glob [I]);
  writeln;
  writeln ('        should be:           DHRYSTONE PROGRAM, 1''ST STRING');
  write   ('String2Glob:                 ');
  for I := 1 to 30 do
    write (String2Glob [I]);
  writeln;
  writeln ('        should be:           DHRYSTONE PROGRAM, 2''ND STRING');
  writeln;
  writeln;

  SumClocks := EndClock - BeginClock;

  if SumClocks < TooSmallTime
    then
    begin
      writeln ('Measured time too small to obtain meaningful results');
      writeln ('Please increase number of runs');
      writeln;
    end
    else
    begin
      Microseconds := SumClocks * (MicrosecondsPerClock / NumberOfRuns);
                                (* Brackets to prevent integer overflow *)
      DhrystonesPerSecond := NumberOfRuns * (ClocksPerSecond / SumClocks);
      write ('Microseconds for one run through Dhrystone: ');
      writeln (Microseconds : 8 : 1);
      write ('Dhrystones per Second:                      ');
      writeln (DhrystonesPerSecond : 8 : 1);
      writeln;
    end;
  
end.
