program funcs(output);
var
  n: integer;
function next(x: integer): integer;
begin
  next := x
end;
function current: integer;
begin
  current := n
end;
begin
  n := next(current);
  n := next(1)
end.
