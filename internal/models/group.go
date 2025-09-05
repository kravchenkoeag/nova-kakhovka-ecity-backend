type Group struct {
ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
Name        string            `bson:"name" json:"name" validate:"required,min=3,max=100"`
Description string            `bson:"description" json:"description" validate:"max=500"`
Type        string            `bson:"type" json:"type" validate:"required,oneof=country region city interest"`

// Фильтры для автодобавления
LocationFilter string   `bson:"location_filter" json:"location_filter"`
InterestFilter []string `bson:"interest_filter" json:"interest_filter"`

// Участники и администраторы
Members     []primitive.ObjectID `bson:"members" json:"members"`
Admins      []primitive.ObjectID `bson:"admins" json:"admins"`
Moderators  []primitive.ObjectID `bson:"moderators" json:"moderators"`

// Настройки
IsPublic     bool `bson:"is_public" json:"is_public"`
AutoJoin     bool `bson:"auto_join" json:"auto_join"`
MaxMembers   int  `bson:"max_members" json:"max_members"`

CreatedAt time.Time `bson:"created_at" json:"created_at"`
UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
CreatedBy primitive.ObjectID `bson:"created_by" json:"created_by"`
}
