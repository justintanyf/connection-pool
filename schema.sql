CREATE TABLE IF NOT EXISTS users
(
    id                  INT unsigned NOT NULL AUTO_INCREMENT,   # Unique User ID
    account             VARCHAR(256) NOT NULL,                  # Account of the User
    nickname            VARCHAR(256) DEFAULT '',                # Nickname of the User
    password            VARCHAR(256) NOT NULL,                  # Password of the User
    pictureFileName     VARCHAR(1024) default '',             # path to the file in the HTTP server
    PRIMARY KEY         (id),                                   # Make the id the primary key
    UNIQUE KEY          (account)
);