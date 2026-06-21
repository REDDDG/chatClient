create table chatroom
(
    id       int auto_increment
        primary key,
    type     int         not null,
    roomName varchar(12) not null
);

create table chatfriends
(
    id         int auto_increment
        primary key,
    userId     int         not null,
    friendId   int         not null,
    roomId     int         null,
    friendName varchar(12) null,
    constraint chatfriends___fk
        foreign key (roomId) references chatroom (id)
);

create index chatfriends_userId_index
    on chatfriends (userId);

create table messages
(
    id          bigint auto_increment
        primary key,
    room_id     int                                      not null,
    sender_id   int                                      not null,
    sender_name varchar(12)                              not null,
    text        varchar(500)                             not null,
    created_at  datetime(3) default CURRENT_TIMESTAMP(3) not null
);

create index idx_room_created
    on messages (room_id, created_at);

create table user
(
    id       int auto_increment
        primary key,
    username varchar(12)             not null,
    password varchar(100)            not null,
    avatar   varchar(255) default '' null,
    constraint username_2
        unique (username)
);

create table user_unread
(
    user_id             int              not null,
    room_id             int              not null,
    first_unread_msg_id bigint default 0 not null,
    unread_count        int    default 0 not null,
    primary key (user_id, room_id)
);

create table userhave
(
    id       int auto_increment
        primary key,
    roomId   int         null,
    roomName varchar(12) null,
    userId   int         null,
    constraint userIn_chatroom_id_fk
        foreign key (roomId) references chatroom (id)
);

create index userin__index
    on userhave (userId);

