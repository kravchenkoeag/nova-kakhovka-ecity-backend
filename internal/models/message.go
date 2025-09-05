type Message struct {
ID       primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
GroupID  primitive.ObjectID `bson:"group_id" json:"group_id" validate:"required"`
UserID   primitive.ObjectID `bson:"user_id" json:"user_id" validate:"required"`

Content   string `bson:"content" json:"content" validate:"required,max=1000"`
Type      string `bson:"type" json:"type" validate:"required,oneof=text image video file link"`

// Медиафайлы
MediaURL    string `bson:"media_url,omitempty" json:"media_url,omitempty"`
MediaType   string `bson:"media_type,omitempty" json:"media_type,omitempty"`
MediaSize   int64  `bson:"media_size,omitempty" json:"media_size,omitempty"`

// Ответ на сообщение
ReplyToID *primitive.ObjectID `bson:"reply_to_id,omitempty" json:"reply_to_id,omitempty"`

// Метаданные
IsEdited  bool      `bson:"is_edited" json:"is_edited"`
IsDeleted bool      `bson:"is_deleted" json:"is_deleted"`
CreatedAt time.Time `bson:"created_at" json:"created_at"`
UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
