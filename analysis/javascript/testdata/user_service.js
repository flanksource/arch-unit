class UserService {
  constructor(database) {
    this.db = database;
    this.cache = new Map();
  }

  async getUser(id) {
    if (this.cache.has(id)) {
      return this.cache.get(id);
    }

    const user = await this.db.findUser(id);
    if (user) {
      this.cache.set(id, user);
    }
    return user;
  }

  createUser(name, email) {
    if (!name || !email) {
      throw new Error("Name and email required");
    }

    const user = {
      id: Date.now(),
      name: name,
      email: email
    };

    this.db.saveUser(user);
    return user;
  }
}

function validateEmail(email) {
  const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return re.test(email);
}

const service = new UserService();
export default service;
