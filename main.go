package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"github.com/c-bata/go-prompt"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"strings"
)

var (
	db       *sql.DB
	task     string
	dbName   string
	dbType   string
	username string
	password []byte
	endpoint string
	schema   string
	uu       []string
	pp       []string
	tt       []string
	allFlag  bool
)

func scanner() string {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		return scanner.Text()
	}

	return ""
}

func mainCompleter(d prompt.Document) []prompt.Suggest {
	s := []prompt.Suggest{
		{Text: "add-user", Description: "Add users to a database"},
		{Text: "add-service-user", Description: "Add a Service user to a database (MySQL only)"},
		{Text: "remove-user", Description: "Remove users to a database"},
	}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func dbTypeCompleter(d prompt.Document) []prompt.Suggest {
	s := []prompt.Suggest{
		{Text: "mysql", Description: ""},
		{Text: "postgres", Description: ""},
	}

	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func main() {
	// Gather all information to be used later.
	err := inputValues()
	if err != nil {
		fmt.Println(err)

		return
	}

	switch task {
	case "add-user":
		err = addUser()
		if err != nil {
			fmt.Println(err)

			return
		}
	case "add-service-user":
		err = addServiceUser()
		if err != nil {
			fmt.Println(err)

			return
		}

	case "remove-user":
		err = removeUser()
		if err != nil {
			fmt.Println(err)

			return
		}
	}
}

func inputValues() error {
	fmt.Println("What would you like to do?")
	task = prompt.Input("> ", mainCompleter)

	fmt.Println("What is the type of the database?")
	dbType = prompt.Input("> ", dbTypeCompleter)

	fmt.Print("What is the endpoint of the RDS instance? ")
	endpoint = scanner()

	fmt.Print("What is the master username? [Press ENTER for: attentive] ")
	username = scanner()
	if username == "" {
		username = "attentive"
	}

	fmt.Print("What is the master password? (input hidden)")
	password, _ = terminal.ReadPassword(0)

	fmt.Print("\nWhat is the database name? [Press ENTER for: attentive] ")
	dbName = scanner()
	if dbName == "" {
		dbName = "attentive"
	}

	err := connect()
	if err != nil {
		return err
	}

	return nil
}

func addUser() error {
	fmt.Print("What are their usernames? (separated by commas) ")
	uu = strings.Split(scanner(), ", ")

	fmt.Print("What are their permissions? (separated by commas) [Press ENTER for SELECT, INSERT, UPDATE]")
	pp = strings.Split(scanner(), ", ")

	fmt.Print("Which tables should they have access to? (separated by commas) [Press ENTER for *]")
	tt = strings.Split(scanner(), ", ")

	allFlag = false

	if len(tt) == 1 {
		allFlag = true
	}

	switch dbType {
	case "mysql":
		err := mysqlAdd()
		if err != nil {
			return err
		}
	case "postgres":
		err := postgresAdd()
		if err != nil {
			return err
		}
	}

	return nil
}

func removeUser() error {
	switch dbType {
	case "mysql":
		err := mysqlRemove()
		if err != nil {
			return err
		}
	case "postgres":
		err := postgresRemove()
		if err != nil {
			return err
		}
	}

	return nil
}

func mysqlRemove() error {
	dropQuery := fmt.Sprintf("DROP USER IF EXSISTS %s", strings.Join(uu, ", "))

	_, err := db.Exec(dropQuery)
	if err != nil {
		return err
	}
	fmt.Println(dropQuery)

	return nil
}

func postgresRemove() error {
	var ss []string

	fmt.Print("What are the schema names? (separated by commas) ")
	ss = strings.Split(scanner(), ", ")

	for _, user := range uu {
		for _, schema := range ss {
			revokeTableQuery := fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s FROM %s;", schema, user)
			revokeSchemaQuery := fmt.Sprintf("REVOKE ALL PRIVILEGES ON SCHEMA %s FROM %s;", schema, user)

			_, err := db.Exec(revokeTableQuery)
			if err != nil {
				return err
			}
			fmt.Println(revokeTableQuery)

			_, err = db.Exec(revokeSchemaQuery)
			if err != nil {
				return err
			}
			fmt.Println(revokeSchemaQuery)
		}

		dropQuery := fmt.Sprintf("DROP USER %s;", user)

		_, err := db.Exec(dropQuery)
		if err != nil {
			return err
		}
		fmt.Println(dropQuery)
	}

	return nil
}

func connect() error {
	var err error

	connString := ""

	switch dbType {
	case "mysql":
		connString = fmt.Sprintf("%s:%s@tcp(%s:3306)/%s", username, password, endpoint, dbName)
	case "postgres":
		connString = fmt.Sprintf("host=%s port=54320 user=%s password=%s dbname=%s sslmode=disable", endpoint, username, password, dbName)
	}

	// Connect to RDS instance to make sure connection succeeds before continuing and save the db connection.
	db, err = sql.Open(dbType, connString)
	if err != nil {
		return err
	}

	fmt.Println("Connection to RDS instance confirmed.")

	return nil
}

func addServiceUser() error {
	fmt.Print("What is the service users username? ")
	serviceUsername := scanner()

	fmt.Print("What is the service users password? ")
	servicePassword := scanner()

	createQuery := fmt.Sprintf("CREATE USER IF NOT EXISTS %s IDENTIFIED BY '%s';", serviceUsername, servicePassword)
	_, err := db.Exec(createQuery)
	if err != nil {
		return err
	}
	fmt.Println(createQuery)

	grantQuery := fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW ON %s.* TO %s;", dbName, serviceUsername)
	_, err = db.Exec(grantQuery)
	if err != nil {
		return err
	}
	fmt.Println(grantQuery)

	return nil
}

func mysqlAdd() error {
	if len(pp) == 1 {
		pp = []string{"SELECT", "INSERT", "UPDATE", "SHOW VIEW"}
	}

	if allFlag {
		tt = []string{"*"}
	}

	// Loop over users and add them to the database with RDS_IAM.
	for _, user := range uu {
		createQuery := fmt.Sprintf("CREATE USER IF NOT EXISTS %s IDENTIFIED BY 'password';", user)
		_, err := db.Exec(createQuery)
		if err != nil {
			return err
		}
		fmt.Println(createQuery)

		// Loop over tables and grant users the permissions that they need.
		for _, table := range tt {
			grantQuery := fmt.Sprintf("GRANT %s ON %s TO %s;", strings.Join(pp, ", "), table, user)
			_, err := db.Exec(grantQuery)
			if err != nil {
				return err
			}
			fmt.Println(grantQuery)
		}
	}

	return nil
}

// handlePostgres handles the postgres database operations.
func postgresAdd() error {
	var ss []string

	fmt.Print("What are the schema names? (separated by commas)")
	ss = strings.Split(scanner(), ", ")

	if len(pp) == 1 {
		// TODO: Postgres-ize this?
		pp = []string{"SELECT", "INSERT", "UPDATE"}
	}

	// Loop over users and add them to the database with RDS_IAM.
USER: // Label for the forloop to continue early within.
	for _, user := range uu {
		createQuery := fmt.Sprintf("CREATE ROLE %s WITH LOGIN; GRANT rds_iam to %s;", user, user)

		_, err := db.Exec(createQuery)
		if err != nil {
			return err
		}
		fmt.Println(createQuery)

		// Grant USAGE to all of the specified schemas.
		for _, schema := range ss {
			usageQuery := fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s;", schema, user)

			_, err := db.Exec(usageQuery)
			if err != nil {
				return err
			}
			fmt.Println(usageQuery)

			if allFlag {
				grantAllQuery := fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s;", strings.Join(pp, ", "), schema, user)
				_, err = db.Exec(grantAllQuery)
				if err != nil {
					return err
				}
				fmt.Println(grantAllQuery)

				continue USER
			}
		}

		for _, table := range tt {
			grantQuery := fmt.Sprintf("GRANT %s ON %s TO %s;", strings.Join(pp, ", "), table, user)
			_, err = db.Exec(grantQuery)
			if err != nil {
				return err
			}
			fmt.Println(grantQuery)
		}
	}

	return nil
}
