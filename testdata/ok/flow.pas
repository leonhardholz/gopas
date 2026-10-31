program flow(output);
var
  n, m: integer;
begin
  if n < 10 then
    n := 1
  else
    writeln('Hello');
  while n < 10 do
    n := n + 1;
  repeat
    n := 1;
    writeln('Hello')
  until n = 1;
  for n := 1 to 10 do
    writeln('Hello');
  for n := 10 downto 1 do
  begin
    m := n
  end;
  case n of
    1: writeln('Hello');
    2, 3: n := 1
  end
end.
