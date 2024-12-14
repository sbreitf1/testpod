package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMakePodName(t *testing.T) {
	require.Equal(t, "testpod-somedude42-20241214-144810", makePodName("somedude42", time.Date(2024, time.December, 14, 14, 48, 10, 0, time.Local)))
	require.Equal(t, "testpod-somedude42-20241215-153700", makePodName("some_dude_42", time.Date(2024, time.December, 15, 15, 37, 0, 0, time.Local)))
	require.Equal(t, "testpod-cool-stuff-20241216-103611", makePodName("cOoL-sTuFf", time.Date(2024, time.December, 16, 10, 36, 11, 0, time.Local)))
	require.Equal(t, "testpod-time-all-ones-20240101-010101", makePodName("time-all-ones", time.Date(2024, time.January, 1, 1, 1, 1, 0, time.Local)))
	require.Equal(t, "testpod-time-all-zero-20240101-000000", makePodName("time-all-zero", time.Date(2024, time.January, 1, 0, 0, 0, 0, time.Local)))
	require.Equal(t, "testpod-e45z4zse5zsnmde4hdeasd6-20240317-041507", makePodName("öüÖüe4ö5z§§4zse5zs_nMDe4hd?ßß`*,;|µe³{asd6", time.Date(2024, time.March, 17, 4, 15, 7, 0, time.Local)))
	require.Equal(t, "testpod-a-far-too-long-hostname-that-tells-a-st-20240317-041507", makePodName("a-far-too-long-hostname-that-tells-a-story-about-unicorns", time.Date(2024, time.March, 17, 4, 15, 7, 0, time.Local)))
	require.Equal(t, "testpod-this-hostname-has-perfectly-fine-length-20240317-041507", makePodName("this-hostname-has-perfectly-fine-length", time.Date(2024, time.March, 17, 4, 15, 7, 0, time.Local)))
	require.Equal(t, "testpod-this-hostname-is-just-one1-char-too-lon-20240317-041507", makePodName("this-hostname-is-just-one1-char-too-long", time.Date(2024, time.March, 17, 4, 15, 7, 0, time.Local)))
}
