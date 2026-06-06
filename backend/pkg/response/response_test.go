package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{"key": "value"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var result map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

func TestWriteJSON_ErrorStatus(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWriteJSON_InvalidData(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, make(chan int))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, "resource not found")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusNotFound), result["code"])
	assert.Equal(t, "resource not found", result["message"])
}

func TestWriteError_InternalServerError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusInternalServerError, "internal error")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestWriteJSON_NoDoubleHeader(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]string{"ok": "true"})

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.Equal(t, `{"ok":"true"}`, w.Body.String())
}

func TestWriteJSON_EmptyObject(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{}", w.Body.String())
}

func TestWriteJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "null", w.Body.String())
}

func TestWriteError_WrapsWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, "test message")
	assert.Equal(t, http.StatusNotFound, w.Code)
}
