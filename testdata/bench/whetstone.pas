program whetstone(output);
{ ISO 7185 form of the double-precision Whetstone loops.
  The Free Pascal source uses sysutils, extended, and longint.
  Array and result parameters are variable parameters, matching
  the original Algol and C benchmarks. Timing is external. }
label
  10, 20, 30, 40, 50, 60, 70, 80, 100;
const
  inner = 100;
  outer = 210; { one execution lasts about a second }
type
  vector = array [1..4] of real;
var
  e1: vector;
  t1, t2, t3, x, y, z: real;
  i, j, k, l, n2, n3, n4, n6, n7, n8, n9, n11, pass: integer;

procedure sub3;
begin
  e1[j] := e1[k];
  e1[k] := e1[l];
  e1[l] := e1[j]
end;

procedure sub2(x, y: real; var z: real);
var
  x1, y1: real;
begin
  x1 := x;
  y1 := y;
  x1 := (x1 + y1) * t1;
  y1 := (x1 + y1) * t1;
  z := (x1 + y1) / t3
end;

procedure sub1(var e: vector);
var
  i: integer;
begin
  for i := 1 to 6 do
  begin
    e[1] := (e[1] + e[2] + e[3] - e[4]) * t1;
    e[2] := (e[1] + e[2] - e[3] + e[4]) * t1;
    e[3] := (e[1] - e[2] + e[3] + e[4]) * t1;
    e[4] := (-e[1] + e[2] + e[3] + e[4]) / t3
  end
end;

begin
  for pass := 1 to outer do
  begin
    t1 := 0.499975;
    t2 := 0.50025;
    t3 := 2.0;
    n2 := 12 * inner;
    n3 := 14 * inner;
    n4 := 345 * inner;
    n6 := 210 * inner;
    n7 := 32 * inner;
    n8 := 899 * inner;
    n9 := 616 * inner;
    n11 := 93 * inner;
    e1[1] := 1.0;
    e1[2] := -1.0;
    e1[3] := -1.0;
    e1[4] := -1.0;
    for i := 1 to n2 do
    begin
      e1[1] := (e1[1] + e1[2] + e1[3] - e1[4]) * t1;
      e1[2] := (e1[1] + e1[2] - e1[3] + e1[4]) * t1;
      e1[3] := (e1[1] - e1[2] + e1[3] + e1[4]) * t1;
      e1[4] := (-e1[1] + e1[2] + e1[3] + e1[4]) * t1
    end;
    for i := 1 to n3 do
      sub1(e1);
    j := 1;
    for i := 1 to n4 do
    begin
      if j - 1 < 0 then
        goto 20;
      if j - 1 = 0 then
        goto 10;
      if j - 1 > 0 then
        goto 20;
      10:
      j := 2;
      goto 30;
      20:
      j := 3;
      30:
      if j - 2 < 0 then
        goto 50;
      if j - 2 = 0 then
        goto 50;
      if j - 2 > 0 then
        goto 40;
      40:
      j := 0;
      goto 60;
      50:
      j := 1;
      60:
      if j - 1 < 0 then
        goto 70;
      if j - 1 = 0 then
        goto 80;
      if j - 1 > 0 then
        goto 80;
      70:
      j := 1;
      goto 100;
      80:
      j := 0;
      100:
    end;
    j := 1;
    k := 2;
    l := 3;
    for i := 1 to n6 do
    begin
      j := j * (k - j) * (l - k);
      k := l * k - (l - j) * k;
      l := (l - k) * (k + j);
      e1[l - 1] := j + k + l;
      e1[k - 1] := j * k * l
    end;
    x := 0.5;
    y := 0.5;
    for i := 1 to n7 do
    begin
      x := t1 * arctan(t3 * sin(y) * cos(y) / (cos(x + y) + cos(x - y) - 1.0));
      y := t1 * arctan(t3 * sin(y) * cos(y) / (cos(x + y) + cos(x - y) - 1.0))
    end;
    x := 1.0;
    y := 1.0;
    z := 1.0;
    for i := 1 to n8 do
      sub2(x, y, z);
    j := 1;
    k := 2;
    l := 3;
    e1[1] := 1.0;
    e1[2] := 2.0;
    e1[3] := 3.0;
    for i := 1 to n9 do
      sub3;
    x := 0.75;
    for i := 1 to n11 do
      x := sqrt(exp(ln(x) / t2))
  end;
  writeln('x = ', x);
  writeln('y = ', y);
  writeln('z = ', z);
  writeln('j = ', j, ' k = ', k, ' l = ', l);
  writeln('e1 = ', e1[1], e1[2], e1[3], e1[4])
end.
