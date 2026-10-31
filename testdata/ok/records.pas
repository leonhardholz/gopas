program records(output);
type
  color = (red, green, blue);
  empty = record
  end;
  date = record
    day: 1..31;
    month: 1..12;
    year: integer
  end;
  person = record
    name: packed array [1..8] of char;
    age: integer;
    case married: boolean of
      true: (spouse: integer);
      false: ()
  end;
  shape = record
    case color of
      red: (x: integer);
      blue, green: ()
  end;
var
  d: date;
  p: person;
  days: array [1..2] of date;
begin
  d.year := 2000;
  p.name[1] := 'a';
  days[1].year := 1;
  with d do
    year := month
end.
