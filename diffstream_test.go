//
// copyright 2023 Scott Means Consulting, LLC DBA CloudTerm Partners
//

package diffstream

import (
	"encoding/json"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/labstack/gommon/log"
)

var td_0 = "Hello, DiffStream"

var td_1 = [...]string{
	"The pen is red.\n",
	"The pen is not red.\n",
	"The pen is blue.\n",
	"The pen is not a pen.\n",
	"This has nothing to do with 'The pen is red.'\n",
	"The pen is not red.\n",
	"The pen is blue.\n",
}

func Test_DiffStream(t *testing.T) {
	log.SetLevel(log.DEBUG)
	ds_0 := New(1)
	ds_0.Channel(0).WriteString(td_0)

	t.Logf("ds_0.Channel(0): %+v", ds_0.Channel(0))

	if ds_0.Channel(0).String() != td_0 {
		t.Fail()
	}

	js, err := json.Marshal(ds_0)

	if err != nil {
		t.Error(err)
	} else {
		t.Logf("ds_0 JSON: %v", string(js))
	}

	ds_1 := New(len(td_1))

	var wg sync.WaitGroup

	for i, s := range td_1 {
		wg.Add(1)

		go func(c int, s string) {
			defer wg.Done()

			for _, r := range s {
				ds_1.Channel(c).WriteRune(r)
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(30)))
			}
		}(i, s)
	}

	wg.Wait()

	t.Logf("test data: %+v", td_1)

	for i := 0; i < ds_1.ChannelCount(); i++ {
		t.Logf("ch_%d: %v", i, ds_1.Channel(i))
		if td_1[i] != ds_1.Channel(i).String() {
			t.Log("^^^^^^^^^^^^^^^^")
			t.Fail()
		}
	}

	t.Logf("final chunks: %v", ds_1.dumpChunks())

	js, err = json.Marshal(ds_1)

	if err != nil {
		t.Error(err)
	} else {
		t.Logf("ds_1 JSON: %v", string(js))
	}
}
