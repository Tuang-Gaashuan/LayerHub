package app
import("testing";"time";"strings")
func TestDryingBlocksDispatch(t *testing.T){s,u,d:=setup(t);p,sp:=fixture(t,s,u);id:=newJob(t,s,u,p,sp,10);d.mu.Lock();d.statuses[p.ID]=Telemetry{Connected:true,Updated:time.Now(),State:"drying",RawState:"FINISH"};d.mu.Unlock();err:=s.DispatchJob(u,id);if err==nil||!strings.Contains(err.Error(),"AMS"){t.Fatalf("expected AMS guard, got %v",err)};if d.uploads!=0{t.Fatal("upload attempted")}}
