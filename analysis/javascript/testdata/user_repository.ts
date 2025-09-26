interface User {
  id: number;
  name: string;
  email: string;
  roles: string[];
}

class UserRepository {
  private users: Map<number, User>;

  constructor() {
    this.users = new Map();
  }

  async findById(id: number): Promise<User | null> {
    const user = this.users.get(id);
    return user || null;
  }

  save(user: User): void {
    if (!user.id) {
      throw new Error("User must have an ID");
    }
    this.users.set(user.id, user);
  }

  findByRole(role: string): User[] {
    const results: User[] = [];
    for (const user of this.users.values()) {
      if (user.roles.includes(role)) {
        results.push(user);
      }
    }
    return results;
  }
}

enum UserRole {
  Admin = "ADMIN",
  User = "USER",
  Guest = "GUEST"
}

type UserWithTimestamp = User & {
  createdAt: Date;
  updatedAt: Date;
};

export { UserRepository, UserRole, UserWithTimestamp };
