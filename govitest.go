package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"
)

// User представляет пользователя с уникальным ID и временем истечения.
type User struct {
    ID        string    `json:"id"`
    ExpiresAt time.Time `json:"expires_at"`
}

// Room представляет конференц-комнату с пользователями.
type Room struct {
    Name     string          `json:"name"`
    Users    map[string]User `json:"users"`
    UserLock sync.RWMutex    // Мьютекс для защиты доступа к пользователям.
}

// ConferenceAPI содержит все комнаты.
type ConferenceAPI struct {
    Rooms    map[string]*Room
    RoomLock sync.RWMutex // Мьютекс для защиты доступа к комнатам.
}

// Инициализируем глобальный экземпляр API.
var api = ConferenceAPI{
    Rooms: make(map[string]*Room),
}

// checkInHandler обрабатывает чек-ин пользователя в комнату.
func checkInHandler(w http.ResponseWriter, r *http.Request) {
    type CheckInRequest struct {
        UserID string `json:"user_id"`
        RoomID string `json:"room_id"`
    }

    var req CheckInRequest

    err := json.NewDecoder(r.Body).Decode(&req)
    if err != nil {
        http.Error(w, "Неверный формат запроса", http.StatusBadRequest)
        return
    }

    if req.UserID == "" || req.RoomID == "" {
        http.Error(w, "Поля user_id и room_id обязательны", http.StatusBadRequest)
        return
    }

    expiresAt := time.Now().Add(5 * time.Minute)

    user := User{
        ID:        req.UserID,
        ExpiresAt: expiresAt,
    }

    api.RoomLock.Lock()
    room, exists := api.Rooms[req.RoomID]
    if !exists {
        room = &Room{
            Name:  req.RoomID,
            Users: make(map[string]User),
        }
        api.Rooms[req.RoomID] = room
    }
    api.RoomLock.Unlock()

    room.UserLock.Lock()
    room.Users[req.UserID] = user
    room.UserLock.Unlock()

    fmt.Fprintf(w, "Пользователь %s зачекинился в комнату %s\n", req.UserID, req.RoomID)
}

// updatePresenceHandler обновляет присутствие пользователя в комнате.
func updatePresenceHandler(w http.ResponseWriter, r *http.Request) {
    type UpdatePresenceRequest struct {
        UserID    string `json:"user_id"`
        RoomID    string `json:"room_id"`
        ExpiresIn int    `json:"expires_in,omitempty"` // Время истечения в секундах.
    }

    var req UpdatePresenceRequest

    err := json.NewDecoder(r.Body).Decode(&req)
    if err != nil {
        http.Error(w, "Неверный формат запроса", http.StatusBadRequest)
        return
    }

    if req.UserID == "" || req.RoomID == "" {
        http.Error(w, "Поля user_id и room_id обязательны", http.StatusBadRequest)
        return
    }

    expiresAt := time.Now().Add(5 * time.Minute)
    if req.ExpiresIn > 0 {
        expiresAt = time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
    }

    api.RoomLock.RLock()
    room, exists := api.Rooms[req.RoomID]
    api.RoomLock.RUnlock()
    if !exists {
        http.Error(w, "Комната не найдена", http.StatusNotFound)
        return
    }

    room.UserLock.Lock()
    user, exists := room.Users[req.UserID]
    if !exists {
        room.UserLock.Unlock()
        http.Error(w, "Пользователь не найден в комнате", http.StatusNotFound)
        return
    }

    user.ExpiresAt = expiresAt
    room.Users[req.UserID] = user
    room.UserLock.Unlock()

    fmt.Fprintf(w, "Присутствие пользователя %s в комнате %s обновлено\n", req.UserID, req.RoomID)
}

// listRoomsHandler возвращает список комнат и их пользователей.
func listRoomsHandler(w http.ResponseWriter, r *http.Request) {
    type RoomInfo struct {
        RoomID  string   `json:"room_id"`
        UserIDs []string `json:"user_ids"`
    }

    var roomsInfo []RoomInfo

    api.RoomLock.RLock()
    for roomID, room := range api.Rooms {
        room.UserLock.Lock()
        var activeUsers []string
        for userID, user := range room.Users {
            if user.ExpiresAt.After(time.Now()) {
                activeUsers = append(activeUsers, userID)
            } else {
                delete(room.Users, userID)
            }
        }
        room.UserLock.Unlock()

        roomsInfo = append(roomsInfo, RoomInfo{
            RoomID:  roomID,
            UserIDs: activeUsers,
        })
    }
    api.RoomLock.RUnlock()

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(roomsInfo)
}

func main() {
    http.HandleFunc("/checkin", checkInHandler)
    http.HandleFunc("/update_presence", updatePresenceHandler)
    http.HandleFunc("/list_rooms", listRoomsHandler)

    fmt.Println("Сервер запущен на порту 8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        fmt.Println("Ошибка запуска сервера:", err)
    }
}
