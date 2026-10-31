program procs(output);
var
  n: integer;
procedure reset;
begin
  n := 0
end;
procedure add(x: integer; var y: integer);
var
  t: integer;
  procedure touch(var z: integer);
  begin
    z := 1
  end;
begin
  touch(y);
  t := x
end;
begin
  reset;
  add(1, n);
  writeln('Hello')
end.
